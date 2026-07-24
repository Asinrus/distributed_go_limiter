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

// AC-41..46 on real Postgres — the same suite the in-memory and Redis backends run.
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
