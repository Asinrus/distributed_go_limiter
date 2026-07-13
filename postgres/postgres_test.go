package pgext_test

import (
	"context"
	"testing"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/asinrus/distributed_go_limiter/core/tck"
	pgext "github.com/asinrus/distributed_go_limiter/postgres"
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
	require.NoError(t, pgext.EnsureSchema(ctx, pool))
	return pool
}

// AC-41..46 on real Postgres — the same suite the in-memory and Redis backends run.
func TestPostgresConformance(t *testing.T) {
	pool := startPostgres(t)
	tck.Run(t, func() *core.ProxyManager {
		return pgext.NewProxyManager(pool)
	})
}

// AC-45: expired state reads as a fresh bucket, and RemoveExpired sweeps the rows.
func TestPostgresExpiration(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	pm := pgext.NewProxyManager(pool, pgext.WithTTL(1*time.Second))
	cfg := core.NewConfig(core.NewBandwidth(10).RefillGreedy(1, time.Hour))

	ok, err := pm.Bucket("exp", cfg).TryConsume(ctx, 4)
	require.NoError(t, err)
	require.True(t, ok)

	time.Sleep(1200 * time.Millisecond)

	av, err := pm.Bucket("exp", cfg).AvailableTokens(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(10), av, "expired state -> fresh full bucket")

	n, err := pgext.NewStore(pool, pgext.WithTTL(1*time.Second)).RemoveExpired(ctx, 100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, int64(1), "at least the expired row is swept")
}
