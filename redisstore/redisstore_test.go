package redisstore_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/asinrus/distributed_go_limiter/core/storetest"
	"github.com/asinrus/distributed_go_limiter/redisstore"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

// fakeClock is a controllable clock for driving refills without a wall-clock
// sleep. core has its own (core/local_test.go) but it is unexported and this
// package sits across a module boundary, so it is re-declared here.
type fakeClock struct{ ns atomic.Int64 }

func newFakeClock() *fakeClock {
	c := &fakeClock{}
	c.ns.Store(time.Now().UnixNano())
	return c
}

func (c *fakeClock) now() int64              { return c.ns.Load() }
func (c *fakeClock) advance(d time.Duration) { c.ns.Add(int64(d)) }

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

// AC-41..44 on real Redis — the same suite the in-memory and Postgres backends run.
func TestRedisConformance(t *testing.T) {
	rdb := startRedis(t)
	storetest.Run(t, func(now func() int64) *core.ProxyManager {
		return redisstore.NewProxyManager(rdb, redisstore.WithProxyOption(core.WithClock(now)))
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

// core.WithClock reaches the Redis constructor via WithProxyOption, so a
// refill can be driven against a real container with no wall-clock sleep —
// the capability T05 is built on.
func TestRedisWithClockDrivesRefillWithNoSleep(t *testing.T) {
	rdb := startRedis(t)
	ctx := context.Background()
	clk := newFakeClock()
	pm := redisstore.NewProxyManager(rdb, redisstore.WithProxyOption(core.WithClock(clk.now)))
	cfg := core.NewConfig(core.NewBandwidth(10).RefillGreedy(10, time.Second))

	b, err := pm.Bucket("clock-refill", cfg)
	require.NoError(t, err)

	ok, err := b.TryConsume(ctx, 10)
	require.NoError(t, err)
	require.True(t, ok, "drain the bucket")

	av, err := b.AvailableTokens(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(0), av)

	clk.advance(time.Second)

	av, err = b.AvailableTokens(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(10), av, "refilled per the injected clock, no wall-clock sleep")
}

// core.WithMaxCASRetries reaches the Redis constructor, and an out-of-range
// value is rejected exactly as on the in-memory manager (AC-55) — proof the
// plumbing is generic, not special-cased for the clock.
func TestRedisMaxCASRetriesReachableAndValidated(t *testing.T) {
	rdb := startRedis(t)
	pm := redisstore.NewProxyManager(rdb, redisstore.WithProxyOption(core.WithMaxCASRetries(0)))
	cfg := core.NewConfig(core.NewBandwidth(10).RefillGreedy(1, time.Hour))

	b, err := pm.Bucket("max-retries", cfg)
	assert.ErrorIs(t, err, core.ErrInvalidConfig)
	assert.Nil(t, b)
}
