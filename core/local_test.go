package core

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClock is a controllable monotonic clock for deterministic tests.
type fakeClock struct{ ns atomic.Int64 }

func (c *fakeClock) now() int64              { return c.ns.Load() }
func (c *fakeClock) advance(d time.Duration) { c.ns.Add(int64(d)) }
func newFakeClock() *fakeClock               { return &fakeClock{} }

func localWithClock(cfg Config, clk *fakeClock) *LocalBucket {
	return newLocalBucketClock(cfg, clk.now)
}

// AC-21: TryConsume behaves per the algorithm through the Bucket API.
func TestAC21_LocalTryConsume(t *testing.T) {
	clk := newFakeClock()
	b := localWithClock(NewConfig(NewBandwidth(10).RefillGreedy(1, time.Second)), clk)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		ok, err := b.TryConsume(ctx, 1)
		require.NoError(t, err)
		assert.True(t, ok, "consume %d", i)
	}
	ok, err := b.TryConsume(ctx, 1)
	require.NoError(t, err)
	assert.False(t, ok, "11th consume must fail")

	_, err = b.TryConsume(ctx, 0)
	assert.ErrorIs(t, err, ErrNonPositiveTokens)
}

// An invalid config (no refill mode set, or non-positive capacity) is rejected at
// construction, so an unusable bucket never exists to be called. A valid config
// still builds.
func TestInvalidConfig_LocalBucket(t *testing.T) {
	t.Run("zero refill period", func(t *testing.T) {
		_, err := NewLocalBucket(NewConfig(NewBandwidth(10))) // no RefillGreedy/RefillIntervally
		assert.ErrorIs(t, err, ErrInvalidConfig)
	})

	// Rejecting at construction is what keeps this consistent: AsBlocking().Consume
	// used to report ErrTokensExceedCapacity here (n > capacity 0) rather than
	// ErrInvalidConfig, because it returns before reaching reserveTokens.
	t.Run("non-positive capacity", func(t *testing.T) {
		_, err := NewLocalBucket(NewConfig(NewBandwidth(0).RefillGreedy(1, time.Second)))
		assert.ErrorIs(t, err, ErrInvalidConfig)
	})

	t.Run("valid config builds", func(t *testing.T) {
		b, err := NewLocalBucket(NewConfig(NewBandwidth(10).RefillGreedy(1, time.Second)))
		require.NoError(t, err)
		require.NotNil(t, b)
	})
}

// AC-22: under concurrency the bucket never over-grants (no data race with -race).
func TestAC22_LocalConcurrencyExactAccounting(t *testing.T) {
	clk := newFakeClock() // frozen: no refill during the run
	const capacity = 1000
	b := localWithClock(NewConfig(NewBandwidth(capacity).RefillGreedy(1, time.Hour)), clk)
	ctx := context.Background()

	var success atomic.Int64
	var wg sync.WaitGroup
	const workers, each = 8, 1000
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < each; i++ {
				if ok, _ := b.TryConsume(ctx, 1); ok {
					success.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(capacity), success.Load(), "exactly capacity consumes succeed")
	avail, _ := b.AvailableTokens(ctx)
	assert.Equal(t, int64(0), avail)
}

// AC-23: AvailableTokens reflects on-read refill as the clock advances.
func TestAC23_AvailableTokensRefills(t *testing.T) {
	clk := newFakeClock()
	b := localWithClock(NewConfig(NewBandwidth(100).RefillGreedy(10, time.Second).WithInitialTokens(0)), clk)
	ctx := context.Background()

	avail, _ := b.AvailableTokens(ctx)
	assert.Equal(t, int64(0), avail)

	clk.advance(5 * time.Second)
	avail, _ = b.AvailableTokens(ctx)
	assert.Equal(t, int64(50), avail)

	clk.advance(100 * time.Second)
	avail, _ = b.AvailableTokens(ctx)
	assert.Equal(t, int64(100), avail, "clamped to capacity")
}

// AC-17: TryConsumeAndReturnRemaining reports remaining tokens and refill wait.
func TestAC17_ConsumptionProbe(t *testing.T) {
	clk := newFakeClock()
	b := localWithClock(NewConfig(NewBandwidth(10).RefillGreedy(10, time.Second)), clk)
	ctx := context.Background()

	p, _ := b.TryConsumeAndReturnRemaining(ctx, 4)
	assert.True(t, p.Consumed)
	assert.Equal(t, int64(6), p.RemainingTokens)

	p, _ = b.TryConsumeAndReturnRemaining(ctx, 6)
	assert.True(t, p.Consumed)
	assert.Equal(t, int64(0), p.RemainingTokens)

	p, _ = b.TryConsumeAndReturnRemaining(ctx, 1)
	assert.False(t, p.Consumed)
	assert.Equal(t, int64(0), p.RemainingTokens)
	assert.Equal(t, 100*int64(time.Millisecond), p.NanosToWaitForRefill) // 1 token @10/s
}

// AC-18: EstimateAbilityToConsume does not consume tokens.
func TestAC18_EstimateNonMutating(t *testing.T) {
	clk := newFakeClock()
	b := localWithClock(NewConfig(NewBandwidth(10).RefillGreedy(10, time.Second).WithInitialTokens(5)), clk)
	ctx := context.Background()

	e, _ := b.EstimateAbilityToConsume(ctx, 3)
	assert.True(t, e.CanBeConsumed)
	assert.Equal(t, int64(5), e.RemainingTokens)

	e, _ = b.EstimateAbilityToConsume(ctx, 8)
	assert.False(t, e.CanBeConsumed)
	assert.Equal(t, int64(5), e.RemainingTokens)
	assert.Equal(t, 300*int64(time.Millisecond), e.NanosToWaitForRefill) // 3 tokens @10/s

	avail, _ := b.AvailableTokens(ctx)
	assert.Equal(t, int64(5), avail, "estimate must not consume")
}

// AC-19: TryConsumeAsMuchAsPossible drains up to the available/limit.
func TestAC19_ConsumeAsMuchAsPossible(t *testing.T) {
	clk := newFakeClock()
	ctx := context.Background()

	b := localWithClock(NewConfig(NewBandwidth(10).RefillGreedy(1, time.Hour).WithInitialTokens(7)), clk)
	got, _ := b.TryConsumeAsMuchAsPossible(ctx, 0) // 0 => all
	assert.Equal(t, int64(7), got)
	avail, _ := b.AvailableTokens(ctx)
	assert.Equal(t, int64(0), avail)

	b = localWithClock(NewConfig(NewBandwidth(10).RefillGreedy(1, time.Hour).WithInitialTokens(7)), clk)
	got, _ = b.TryConsumeAsMuchAsPossible(ctx, 3) // capped at limit
	assert.Equal(t, int64(3), got)
	avail, _ = b.AvailableTokens(ctx)
	assert.Equal(t, int64(4), avail)
}
