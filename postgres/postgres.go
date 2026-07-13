// Package pgext is the Postgres storage backend for the distributed rate limiter.
// It implements core.LockStore using SELECT ... FOR UPDATE: the row lock is held
// across the read-modify-write, so concurrent updaters serialize with no CAS loop.
// The token math runs in core. See docs/project/EXT_POSTGRES.md.
package pgext

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
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

// keyToID maps a string key to the BIGINT primary key (FNV-64a). v1 supports
// BIGINT-keyed tables; VARCHAR keys are a possible future option (see QUESTIONS).
func keyToID(key string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int64(h.Sum64())
}

const maxLockRetries = 16

func (s *Store) Do(ctx context.Context, key string, ttl time.Duration,
	fn func(old []byte, found bool) ([]byte, bool, error)) error {
	id := keyToID(key)
	for i := 0; i < maxLockRetries; i++ {
		done, err := s.doOnce(ctx, id, ttl, fn)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		// placeholder row was inserted; retry so it can be locked
	}
	return errors.New("pgext: exceeded retries acquiring row lock")
}

func (s *Store) doOnce(ctx context.Context, id int64, ttl time.Duration,
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
	scanErr := tx.QueryRow(ctx, s.selectSQL, id).Scan(&state, &expiresAt)
	if errors.Is(scanErr, pgx.ErrNoRows) {
		// Row doesn't exist yet — insert a placeholder we can lock next round.
		if _, err = tx.Exec(ctx, s.insertSQL, id); err != nil {
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
		if _, err = tx.Exec(ctx, s.updateSQL, newState, exp, id); err != nil {
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
	_, err := s.pool.Exec(ctx, s.deleteSQL, keyToID(key))
	return err
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
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// EnsureSchema creates the bucket table if it does not exist.
func EnsureSchema(ctx context.Context, pool *pgxpool.Pool, opts ...Option) error {
	c := newConfig(opts...)
	ddl := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (%s BIGINT PRIMARY KEY, %s BYTEA, expires_at BIGINT)`,
		c.table, c.idCol, c.stateCol)
	_, err := pool.Exec(ctx, ddl)
	return err
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
