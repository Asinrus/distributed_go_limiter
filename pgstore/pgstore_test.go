package pgstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/asinrus/distributed_go_limiter/core/tck"
	"github.com/asinrus/distributed_go_limiter/pgstore"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping container test in -short mode")
	}
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.MaxConns = 20 // enough headroom for the contention tests
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	require.NoError(t, pool.Ping(ctx))
	require.NoError(t, pgstore.EnsureSchema(ctx, pool))
	return pool
}

// AC-41..44 on real Postgres — the same suite the in-memory and Redis backends run.
func TestPostgresConformance(t *testing.T) {
	pool := startPostgres(t)
	tck.Run(t, func() *core.ProxyManager {
		return pgstore.NewProxyManager(pool)
	})
}

// AC-45: expired state reads as a fresh bucket, and RemoveExpired sweeps the rows.
func TestPostgresExpiration(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	pm := pgstore.NewProxyManager(pool, pgstore.WithTTL(1*time.Second))
	cfg := core.NewConfig(core.NewBandwidth(10).RefillGreedy(1, time.Hour))

	b, err := pm.Bucket("exp", cfg)
	require.NoError(t, err)

	ok, err := b.TryConsume(ctx, 4)
	require.NoError(t, err)
	require.True(t, ok)

	time.Sleep(1200 * time.Millisecond)

	av, err := b.AvailableTokens(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(10), av, "expired state -> fresh full bucket")

	n, err := pgstore.NewStore(pool, pgstore.WithTTL(1*time.Second)).RemoveExpired(ctx, 100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, int64(1), "at least the expired row is swept")
}

// Table/column names reach SQL by interpolation, not by bind parameter. They are
// application config rather than end-user input, but a hostile identifier must be
// escaped, never executed — and every interpolating statement must agree on the
// escaping, including the derived index name and the information_schema guard.
func TestAC47_HostileIdentifiersAreEscapedNotExecuted(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	_, err := pool.Exec(ctx, `CREATE TABLE sentinel (x INT)`)
	require.NoError(t, err)

	// Every identifier closes the quote and appends a second statement.
	var (
		table = `b"; DROP TABLE sentinel; --`
		idCol = `i"d`
		stCol = `st"ate`
		opts  = []pgstore.Option{
			pgstore.WithTable(table),
			pgstore.WithIDColumn(idCol),
			pgstore.WithStateColumn(stCol),
		}
	)

	require.NoError(t, pgstore.EnsureSchema(ctx, pool, opts...))
	require.NoError(t, pgstore.EnsureSchema(ctx, pool, opts...), "idempotent")

	// The full read-modify-write path (select/insert/update) uses those identifiers.
	pm := pgstore.NewProxyManager(pool, append(opts, pgstore.WithTTL(time.Hour))...)
	b, err := pm.Bucket("k", core.NewConfig(core.NewBandwidth(2).RefillGreedy(1, time.Hour)))
	require.NoError(t, err)
	ok, err := b.TryConsume(ctx, 1)
	require.NoError(t, err)
	require.True(t, ok)

	av, err := b.AvailableTokens(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), av)

	// RemoveExpired and Remove interpolate too.
	store := pgstore.NewStore(pool, opts...)
	_, err = store.RemoveExpired(ctx, 10)
	require.NoError(t, err)
	require.NoError(t, store.Remove(ctx, "k"))

	// Nothing detonated: the injected statement never ran, and the table really is
	// the one whose name contains a quote.
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM sentinel`).Scan(&n))
	assert.Equal(t, 0, n)

	var exists bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.columns
		                WHERE table_name = $1 AND column_name = $2)`,
		table, idCol).Scan(&exists))
	assert.True(t, exists, "quoted identifiers are stored verbatim")

	// The BIGINT guard must still fire for a quoted name: it looks the column up in
	// information_schema, which only matches if DDL and lookup normalize the same way.
	_, err = pool.Exec(ctx, `CREATE TABLE "Legacy" ("Id" BIGINT PRIMARY KEY, state BYTEA, expires_at BIGINT)`)
	require.NoError(t, err)
	err = pgstore.EnsureSchema(ctx, pool, pgstore.WithTable("Legacy"), pgstore.WithIDColumn("Id"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "want text")
}

// SchemaSQL is the migration path for users who own their own DDL: running its
// statements by hand must produce exactly the table EnsureSchema would have made,
// and identifiers must be quoted there too.
func TestSchemaSQLIsAppliableByHand(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	opts := []pgstore.Option{pgstore.WithTable("Manual"), pgstore.WithIDColumn("Key")}
	stmts := pgstore.SchemaSQL(opts...)
	require.Len(t, stmts, 2)
	assert.Contains(t, stmts[0], `"Manual"`, "identifiers are quoted, not folded")
	assert.Contains(t, stmts[0], `"Key" TEXT PRIMARY KEY`)

	for _, s := range stmts {
		_, err := pool.Exec(ctx, s)
		require.NoError(t, err)
	}

	// EnsureSchema accepts the hand-applied table (its TEXT guard passes), and the
	// store works against it end to end.
	require.NoError(t, pgstore.EnsureSchema(ctx, pool, opts...))

	pm := pgstore.NewProxyManager(pool, opts...)
	b, err := pm.Bucket("k", core.NewConfig(core.NewBandwidth(2).RefillGreedy(1, time.Hour)))
	require.NoError(t, err)
	ok, err := b.TryConsume(ctx, 1)
	require.NoError(t, err)
	require.True(t, ok)
}

// Keys are TEXT. A table left over from an earlier BIGINT-keyed build survives
// CREATE TABLE IF NOT EXISTS untouched, so EnsureSchema must report it instead of
// letting every later query fail with an opaque type error.
func TestEnsureSchemaRejectsLegacyBigintTable(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	_, err := pool.Exec(ctx,
		`CREATE TABLE legacy_bucket (id BIGINT PRIMARY KEY, state BYTEA, expires_at BIGINT)`)
	require.NoError(t, err)

	err = pgstore.EnsureSchema(ctx, pool, pgstore.WithTable("legacy_bucket"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "want text")

	// A fresh table name still goes through cleanly, and is idempotent.
	require.NoError(t, pgstore.EnsureSchema(ctx, pool, pgstore.WithTable("fresh_bucket")))
	require.NoError(t, pgstore.EnsureSchema(ctx, pool, pgstore.WithTable("fresh_bucket")))
}
