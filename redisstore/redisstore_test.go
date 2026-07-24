package redisstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/asinrus/distributed_go_limiter/core/tck"
	"github.com/asinrus/distributed_go_limiter/redisstore"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func startRedis(t *testing.T) *redis.Client {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping container test in -short mode")
	}
	ctx := context.Background()
	ctr, err := tcredis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	uri, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)
	opt, err := redis.ParseURL(uri)
	require.NoError(t, err)
	rdb := redis.NewClient(opt)
	t.Cleanup(func() { _ = rdb.Close() })
	require.NoError(t, rdb.Ping(ctx).Err())
	return rdb
}

// AC-41..46 on real Redis — the same suite the in-memory and Postgres backends run.
func TestRedisConformance(t *testing.T) {
	rdb := startRedis(t)
	tck.Run(t, func() *core.ProxyManager {
		return redisstore.NewProxyManager(rdb)
	})
}

// AC-45: expire-after-write on Redis (short TTL + real wait).
func TestRedisExpiration(t *testing.T) {
	rdb := startRedis(t)
	ctx := context.Background()
	pm := redisstore.NewProxyManager(rdb, redisstore.WithTTL(1*time.Second))
	cfg := core.NewConfig(core.NewBandwidth(10).RefillGreedy(1, time.Hour))

	b, err := pm.Bucket("exp", cfg)
	require.NoError(t, err)

	ok, err := b.TryConsume(ctx, 4)
	require.NoError(t, err)
	require.True(t, ok)

	av, err := b.AvailableTokens(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(6), av)

	time.Sleep(1500 * time.Millisecond)

	av, err = b.AvailableTokens(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(10), av, "state expired -> fresh full bucket")
}
