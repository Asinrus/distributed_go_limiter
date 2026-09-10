// Package pgstore is the Postgres storage backend for the distributed rate limiter.
// It implements core.LockStore using SELECT ... FOR UPDATE: the row lock is held
// across the read-modify-write, so concurrent updaters serialize with no CAS loop.
// The token math runs in core. See docs/client/postgres.md.
package pgstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type config struct {
	table    string
	idCol    string
	stateCol string
	coreOpts []core.ProxyOption
}

func defaultConfig() config {
	return config{table: "bucket", idCol: "id", stateCol: "state"}
}

// Option configures the Postgres backend.
type Option func(*config)

// WithTable sets the bucket table name (default "bucket"). The value is a single
// identifier, quoted verbatim — case-sensitive and not schema-qualifiable. For a
// non-default schema, set search_path on the connection and pass a bare table name.
func WithTable(t string) Option { return func(c *config) { c.table = t } }

// WithIDColumn sets the primary-key column name (default "id").
func WithIDColumn(col string) Option { return func(c *config) { c.idCol = col } }

// WithStateColumn sets the state (BYTEA) column name (default "state").
func WithStateColumn(col string) Option { return func(c *config) { c.stateCol = col } }

// WithTTL sets an expire-after-write TTL (0 = no expiry). It is sugar for
// WithProxyOption(core.WithExpireAfterWrite(d)) — the TTL must reach the store
// as well as core.ProxyManager's bookkeeping, so this is where both happen. If
// both WithTTL and an explicit core.WithExpireAfterWrite (passed via
// WithProxyOption) are given to NewProxyManager, whichever is passed last wins,
// same as any functional option.
func WithTTL(d time.Duration) Option {
	return func(c *config) { c.coreOpts = append(c.coreOpts, core.WithExpireAfterWrite(d)) }
}

// WithProxyOption forwards a core.ProxyOption — core.WithClock,
// core.WithMaxCASRetries, core.WithExpireAfterWrite, or any future one — to the
// underlying core.ProxyManager, so nothing core exposes is unreachable from this
// constructor. See WithTTL's doc for the case where both set a TTL.
func WithProxyOption(o core.ProxyOption) Option {
	return func(c *config) { c.coreOpts = append(c.coreOpts, o) }
}

func newConfig(opts ...Option) config {
	c := defaultConfig()
	for _, o := range opts {
		o(&c)
	}
	return c
}

// Store is the Postgres core.LockStore. Obtain one with NewStore; most callers
// want NewProxyManager instead.
type Store struct {
	pool *pgxpool.Pool
	cfg  config
	now  func() int64

	selectSQL, insertSQL, updateSQL, deleteSQL string
}

var _ core.LockStore = (*Store)(nil)

// quote renders a configured name as a single SQL identifier: double-quoted, with
// embedded quotes escaped. Identifiers cannot be bind parameters, so this is the only
// thing standing between WithTable/WithIDColumn/WithStateColumn and the SQL parser.
// Quoting also makes the name case-sensitive and exact, which is what lets the
// information_schema lookup in EnsureSchema compare against the raw value.
func quote(name string) string { return pgx.Identifier{name}.Sanitize() }

func newStore(pool *pgxpool.Pool, c config) *Store {
	t, id, st := quote(c.table), quote(c.idCol), quote(c.stateCol)
	return &Store{
		pool: pool,
		cfg:  c,
		now:  func() int64 { return time.Now().UnixNano() },
		// state may be NULL (a row leaked by a build predating AC-48); expires_at NULL
		// = no expiry.
		selectSQL: fmt.Sprintf(`SELECT %s, expires_at FROM %s WHERE %s = $1 FOR UPDATE`, st, t, id),
		insertSQL: fmt.Sprintf(`INSERT INTO %s (%s, %s, expires_at) VALUES ($1, $2, $3) ON CONFLICT (%s) DO NOTHING`, t, id, st, id),
		updateSQL: fmt.Sprintf(`UPDATE %s SET %s = $1, expires_at = $2 WHERE %s = $3`, t, st, id),
		deleteSQL: fmt.Sprintf(`DELETE FROM %s WHERE %s = $1`, t, id),
	}
}

const maxLockRetries = 16

func (s *Store) Do(ctx context.Context, key string, ttl time.Duration,
	fn func(old []byte, found bool) ([]byte, bool, error)) error {
	for i := 0; i < maxLockRetries; i++ {
		done, err := s.doOnce(ctx, key, ttl, fn)
		if err != nil {
			return fmt.Errorf("pgstore: do %q: %w", key, err)
		}
		if done {
			return nil
		}
		// a concurrent writer created the row first; retry so it can be locked
	}
	return fmt.Errorf("pgstore: %q: exceeded retries acquiring row lock", key)
}

func (s *Store) doOnce(ctx context.Context, key string, ttl time.Duration,
	fn func(old []byte, found bool) ([]byte, bool, error)) (done bool, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	var state []byte
	var expiresAt *int64
	scanErr := tx.QueryRow(ctx, s.selectSQL, key).Scan(&state, &expiresAt)
	rowExists := !errors.Is(scanErr, pgx.ErrNoRows)
	if rowExists && scanErr != nil {
		return false, scanErr
	}

	// found=false for a missing row, a leaked NULL-state row (predates AC-48), and an
	// expired one.
	found := rowExists && state != nil
	if found && expiresAt != nil && *expiresAt <= s.now() {
		found = false
		state = nil
	}

	newState, write, ferr := fn(state, found)
	if ferr != nil {
		return false, ferr
	}
	if !write {
		// No statement, rollback releases the transaction. Nothing is persisted —
		// parity with Redis/memstore on a read of an absent/found key.
		return true, nil
	}

	if !rowExists {
		tag, err := tx.Exec(ctx, s.insertSQL, key, newState, s.expiresAt(ttl))
		if err != nil {
			return false, err
		}
		if tag.RowsAffected() == 0 {
			// Another writer created the key first; newState was derived from a state
			// we never actually read, so it must be discarded. Retry: the next
			// SELECT ... FOR UPDATE will find the row that writer created.
			return false, nil
		}
	} else if _, err = tx.Exec(ctx, s.updateSQL, newState, s.expiresAt(ttl), key); err != nil {
		return false, err
	}

	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	committed = true
	return true, nil
}

// expiresAt computes the expires_at value for a write: now()+ttl, or nil (no expiry).
func (s *Store) expiresAt(ttl time.Duration) *int64 {
	if ttl <= 0 {
		return nil
	}
	e := s.now() + ttl.Nanoseconds()
	return &e
}

func (s *Store) Remove(ctx context.Context, key string) error {
	if _, err := s.pool.Exec(ctx, s.deleteSQL, key); err != nil {
		return fmt.Errorf("pgstore: remove %q: %w", key, err)
	}
	return nil
}

// RemoveExpired deletes up to batch expired rows and returns how many were removed.
func (s *Store) RemoveExpired(ctx context.Context, batch int) (int64, error) {
	q := fmt.Sprintf(
		`DELETE FROM %[1]s WHERE %[2]s IN (
            SELECT %[2]s FROM %[1]s WHERE expires_at IS NOT NULL AND expires_at <= $1
            ORDER BY %[2]s FOR UPDATE SKIP LOCKED LIMIT $2)`,
		quote(s.cfg.table), quote(s.cfg.idCol))
	tag, err := s.pool.Exec(ctx, q, s.now(), batch)
	if err != nil {
		return 0, fmt.Errorf("pgstore: remove expired: %w", err)
	}
	return tag.RowsAffected(), nil
}

// SchemaSQL returns the DDL statements for the bucket table and its expiry index,
// in the order they must run, for callers who apply migrations with their own
// tooling (goose, Flyway, atlas, a hand-written .sql file) instead of EnsureSchema.
// Pass the same Options the Store is built with, or the names will not match.
func SchemaSQL(opts ...Option) []string {
	c := newConfig(opts...)
	return []string{
		fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %[1]s (%[2]s TEXT PRIMARY KEY, %[3]s BYTEA, expires_at BIGINT)`,
			quote(c.table), quote(c.idCol), quote(c.stateCol)),
		// The index name is derived, so it is one identifier built from the table name —
		// quote the concatenation, not the parts.
		fmt.Sprintf(
			`CREATE INDEX IF NOT EXISTS %[1]s ON %[2]s (expires_at) WHERE expires_at IS NOT NULL`,
			quote(c.table+"_expires_at_idx"), quote(c.table)),
	}
}

// EnsureSchema applies SchemaSQL against pool, so a program can create its own
// table on boot. It does NOT migrate an existing table: keys are TEXT, and a table
// left over from an earlier BIGINT-keyed build is reported as an error rather than
// silently surviving CREATE TABLE IF NOT EXISTS and failing on every query.
func EnsureSchema(ctx context.Context, pool *pgxpool.Pool, opts ...Option) error {
	c := newConfig(opts...)

	// Compared against the raw names, not the quoted ones: the DDL quotes every
	// identifier, so what lands in the catalog is the configured string verbatim. Keep
	// the two in step — dropping the quoting would reintroduce Postgres's fold-to-
	// lowercase and this guard would stop matching for any non-lowercase name.
	const colTypeQ = `SELECT data_type FROM information_schema.columns
	                  WHERE table_name = $1 AND column_name = $2`
	var dt string
	switch err := pool.QueryRow(ctx, colTypeQ, c.table, c.idCol).Scan(&dt); {
	case errors.Is(err, pgx.ErrNoRows): // table doesn't exist yet — nothing to clash with
	case err != nil:
		return fmt.Errorf("pgstore: inspect %s.%s: %w", c.table, c.idCol, err)
	case dt != "text":
		return fmt.Errorf("pgstore: %s.%s is %s, want text; drop and recreate the table",
			c.table, c.idCol, dt)
	}

	for _, stmt := range SchemaSQL(opts...) {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("pgstore: ensure schema: %w", err)
		}
	}
	return nil
}

// NewStore returns the raw LockStore (useful for RemoveExpired). Most callers want
// NewProxyManager instead.
func NewStore(pool *pgxpool.Pool, opts ...Option) *Store {
	return newStore(pool, newConfig(opts...))
}

// NewProxyManager builds a distributed ProxyManager backed by Postgres. Call
// EnsureSchema first (or create the table yourself). Calling code uses the returned
// core.ProxyManager exactly like any other backend.
func NewProxyManager(pool *pgxpool.Pool, opts ...Option) *core.ProxyManager {
	c := newConfig(opts...)
	store := newStore(pool, c)
	return core.NewLockProxyManager(store, c.coreOpts...)
}
