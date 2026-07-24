// Package pgstore is the Postgres storage backend for the distributed rate limiter.
// It implements core.LockStore using SELECT ... FOR UPDATE: the row lock is held
// across the read-modify-write, so concurrent updaters serialize with no CAS loop.
// The token math runs in core. See docs/internal/project/EXT_POSTGRES.md.
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
	ttl      time.Duration
}

func defaultConfig() config {
	return config{table: "bucket", idCol: "id", stateCol: "state"}
}

// Option configures the Postgres backend.
type Option func(*config)

// WithTable sets the bucket table name (default "bucket").
func WithTable(t string) Option { return func(c *config) { c.table = t } }

// WithIDColumn sets the primary-key column name (default "id").
func WithIDColumn(col string) Option { return func(c *config) { c.idCol = col } }

// WithStateColumn sets the state (BYTEA) column name (default "state").
func WithStateColumn(col string) Option { return func(c *config) { c.stateCol = col } }

// WithTTL sets an expire-after-write TTL (0 = no expiry).
func WithTTL(d time.Duration) Option { return func(c *config) { c.ttl = d } }

func newConfig(opts ...Option) config {
	c := defaultConfig()
	for _, o := range opts {
		o(&c)
	}
	return c
}

type Store struct {
	pool *pgxpool.Pool
	cfg  config
	now  func() int64

	selectSQL, insertSQL, updateSQL, deleteSQL string
}

var _ core.LockStore = (*Store)(nil)

func newStore(pool *pgxpool.Pool, c config) *Store {
	t, id, st := c.table, c.idCol, c.stateCol
	return &Store{
		pool: pool,
		cfg:  c,
		now:  func() int64 { return time.Now().UnixNano() },
		// state may be NULL (placeholder row); expires_at NULL = no expiry.
		selectSQL: fmt.Sprintf(`SELECT %s, expires_at FROM %s WHERE %s = $1 FOR UPDATE`, st, t, id),
		insertSQL: fmt.Sprintf(`INSERT INTO %s (%s, %s) VALUES ($1, NULL) ON CONFLICT (%s) DO NOTHING`, t, id, st, id),
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
		// placeholder row was inserted; retry so it can be locked
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
	if errors.Is(scanErr, pgx.ErrNoRows) {
		// Row doesn't exist yet — insert a placeholder we can lock next round.
		if _, err = tx.Exec(ctx, s.insertSQL, key); err != nil {
			return false, err
		}
		if err = tx.Commit(ctx); err != nil {
			return false, err
		}
		committed = true
		return false, nil // retry in a fresh transaction
	}
	if scanErr != nil {
		return false, scanErr
	}

	// Treat an expired row as absent (fresh bucket).
	found := state != nil
	if found && expiresAt != nil && *expiresAt <= s.now() {
		found = false
		state = nil
	}

	newState, write, ferr := fn(state, found)
	if ferr != nil {
		return false, ferr
	}
	if write {
		var exp *int64
		if ttl > 0 {
			e := s.now() + ttl.Nanoseconds()
			exp = &e
		}
		if _, err = tx.Exec(ctx, s.updateSQL, newState, exp, key); err != nil {
			return false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	committed = true
	return true, nil
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
		s.cfg.table, s.cfg.idCol)
	tag, err := s.pool.Exec(ctx, q, s.now(), batch)
	if err != nil {
		return 0, fmt.Errorf("pgstore: remove expired: %w", err)
	}
	return tag.RowsAffected(), nil
}

// EnsureSchema creates the bucket table (and its expiry index) if they do not
// exist. It does NOT migrate an existing table: keys are TEXT, and a table left
// over from an earlier BIGINT-keyed build is reported as an error rather than
// silently surviving CREATE TABLE IF NOT EXISTS and failing on every query.
func EnsureSchema(ctx context.Context, pool *pgxpool.Pool, opts ...Option) error {
	c := newConfig(opts...)

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

	ddl := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %[1]s (%[2]s TEXT PRIMARY KEY, %[3]s BYTEA, expires_at BIGINT)`,
		c.table, c.idCol, c.stateCol)
	if _, err := pool.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("pgstore: ensure schema: %w", err)
	}
	idx := fmt.Sprintf(
		`CREATE INDEX IF NOT EXISTS %[1]s_expires_at_idx ON %[1]s (expires_at) WHERE expires_at IS NOT NULL`,
		c.table)
	if _, err := pool.Exec(ctx, idx); err != nil {
		return fmt.Errorf("pgstore: ensure schema index: %w", err)
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
	return core.NewLockProxyManager(store, core.WithExpireAfterWrite(c.ttl))
}
