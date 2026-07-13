package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC-31: Consume blocks ~the computed wait, then succeeds.
func TestAC31_BlockingConsumeWaits(t *testing.T) {
	clk := newFakeClock()
	// 100 tokens/s, empty: 5 tokens need 50ms.
	b := localWithClock(NewConfig(NewBandwidth(100).RefillGreedy(100, time.Second).WithInitialTokens(0)), clk)

	start := time.Now()
	err := b.AsBlocking().Consume(context.Background(), 5)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, elapsed, 40*time.Millisecond)
	assert.Less(t, elapsed, 500*time.Millisecond)
}

// AC-32: Consume respects a deadline it cannot meet — no reservation, returns fast.
func TestAC32_BlockingConsumeDeadlineExceeded(t *testing.T) {
	clk := newFakeClock()
	// 1 token/s, empty: 50 tokens need 50s — far beyond the deadline.
	b := localWithClock(NewConfig(NewBandwidth(100).RefillGreedy(1, time.Second).WithInitialTokens(0)), clk)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := b.AsBlocking().Consume(ctx, 50)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 40*time.Millisecond, "should not sleep the full wait")

	avail, _ := b.AvailableTokens(context.Background())
	assert.Equal(t, int64(0), avail, "nothing reserved when the deadline can't be met")
}

// AC-32: cancellation while waiting returns ctx.Err() promptly.
func TestAC32_BlockingConsumeCancelledMidWait(t *testing.T) {
	clk := newFakeClock()
	// 100 tokens/s, empty: 50 tokens need 500ms.
	b := localWithClock(NewConfig(NewBandwidth(100).RefillGreedy(100, time.Second).WithInitialTokens(0)), clk)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := b.AsBlocking().Consume(ctx, 50)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(start), 300*time.Millisecond)
}

// n exceeding capacity can never be satisfied.
func TestBlockingConsumeExceedsCapacity(t *testing.T) {
	clk := newFakeClock()
	b := localWithClock(NewConfig(NewBandwidth(10).RefillGreedy(1, time.Second)), clk)
	err := b.AsBlocking().Consume(context.Background(), 11)
	assert.ErrorIs(t, err, ErrTokensExceedCapacity)
}
