package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC-53: AvailableTokens never exposes a negative count, even though the
// internal ledger goes negative for the duration of a blocking reservation.
// ai-context/tasks/core-available-tokens-negative.md.

// TestAC53_LocalBlockingReservationClampsToZero is the repro: a LocalBucket
// drained to empty, one goroutine reserving via AsBlocking().Consume while a
// second goroutine polls AvailableTokens throughout. Run under -race.
func TestAC53_LocalBlockingReservationClampsToZero(t *testing.T) {
	clk := newFakeClock()
	// 10 tokens/s, empty: 5 tokens need 500ms to reserve.
	b := localWithClock(NewConfig(NewBandwidth(10).RefillGreedy(10, time.Second).WithInitialTokens(0)), clk)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = b.AsBlocking().Consume(context.Background(), 5)
	}()

	deadline := time.Now().Add(700 * time.Millisecond)
	for time.Now().Before(deadline) {
		avail, err := b.AvailableTokens(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(0), avail, "must never observe a negative available count")
		select {
		case <-done:
			return
		default:
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Consume did not finish in time")
}

// The same repro through the distributed path, which additionally exercises the
// encode/decode round trip of a negative bucketState through core/codec.go.
func TestAC53_DistributedBlockingReservationClampsToZero(t *testing.T) {
	clk := newFakeClock()
	pm := NewInMemoryProxyManager(WithClock(clk.now))
	b, err := pm.Bucket("k", NewConfig(NewBandwidth(10).RefillGreedy(10, time.Second).WithInitialTokens(0)))
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = b.AsBlocking().Consume(context.Background(), 5)
	}()

	deadline := time.Now().Add(700 * time.Millisecond)
	for time.Now().Before(deadline) {
		avail, err := b.AvailableTokens(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(0), avail, "must never observe a negative available count")
		select {
		case <-done:
			return
		default:
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Consume did not finish in time")
}

// The reservation itself is unaffected by the clamp: Consume still waits the
// computed delay and still deducts the tokens it reserved. A clamp that leaked
// into the ledger would show up here as a waiter served early or twice.
func TestAC53_ReservationStillWaitsAndConsumes(t *testing.T) {
	clk := newFakeClock()
	b := localWithClock(NewConfig(NewBandwidth(10).RefillGreedy(10, time.Second).WithInitialTokens(0)), clk)

	start := time.Now()
	err := b.AsBlocking().Consume(context.Background(), 5)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, elapsed, 400*time.Millisecond)

	avail, err := b.AvailableTokens(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), avail, "the 5 tokens were actually spent")
}

// A merely-empty bucket (no reservation) still reports exactly 0, and a
// partially filled bucket still reports its real count: the clamp must not
// become a floor that hides a wrong positive value.
func TestAC53_ClampIsNotAFloor(t *testing.T) {
	clk := newFakeClock()

	empty := localWithClock(NewConfig(NewBandwidth(10).RefillGreedy(1, time.Second).WithInitialTokens(0)), clk)
	avail, err := empty.AvailableTokens(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), avail)

	partial := localWithClock(NewConfig(NewBandwidth(10).RefillGreedy(1, time.Second).WithInitialTokens(4)), clk)
	avail, err = partial.AvailableTokens(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(4), avail)
}
