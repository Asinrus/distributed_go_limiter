package core

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sec = int64(time.Second) // 1e9 ns

// AC-11: consume up to capacity, then no tokens remain and a wait is required.
func TestAC11_ConsumeToCapacityThenEmpty(t *testing.T) {
	s := newState(NewBandwidth(10).RefillGreedy(10, time.Second), 0)
	assert.Equal(t, int64(10), s.available())

	s.consume(10)
	assert.Equal(t, int64(0), s.available())
	// 1 token at 10/s == 100ms.
	assert.Equal(t, 100*int64(time.Millisecond), s.delayNanosForConsume(1, 0))
}

// AC-12: greedy refill adds tokens proportionally to elapsed time.
func TestAC12_GreedyRefillRate(t *testing.T) {
	bw := NewBandwidth(1_000_000).RefillGreedy(10, time.Second).WithInitialTokens(0)

	// Exact multiples: k periods -> k*refillTokens, rounding error 0.
	for _, k := range []int64{1, 3, 7} {
		s := newState(bw, 0)
		s.refill(k * sec)
		assert.Equal(t, k*10, s.available(), "after %d periods", k)
		assert.Equal(t, int64(0), s.roundingError, "no carry at exact multiple")
	}

	// Sub-period: 1.5 periods of 10 tok/s -> 15 tokens.
	s := newState(bw, 0)
	s.refill(sec + sec/2)
	assert.Equal(t, int64(15), s.available())
}

// AC-13: intervally refill only lands on whole-period boundaries.
func TestAC13_IntervallyRefillOnBoundaries(t *testing.T) {
	bw := NewBandwidth(1_000_000).RefillIntervally(10, time.Second).WithInitialTokens(0)

	s := newState(bw, 0)
	s.refill(sec + 9*sec/10) // 1.9 periods -> only 1 boundary crossed
	assert.Equal(t, int64(10), s.available())

	s = newState(bw, 0)
	s.refill(2*sec + sec/2) // 2.5 periods -> 2 boundaries
	assert.Equal(t, int64(20), s.available())
}

// AC-14: refill never exceeds capacity, even for enormous elapsed time.
func TestAC14_RefillClampsToCapacity(t *testing.T) {
	s := newState(NewBandwidth(10).RefillGreedy(10, time.Second).WithInitialTokens(0), 0)
	s.refill(1000 * sec)
	assert.Equal(t, int64(10), s.available())

	// Near-overflow elapsed time still clamps, never wraps negative.
	s = newState(NewBandwidth(10).RefillGreedy(10, time.Second).WithInitialTokens(0), 0)
	s.refill(math.MaxInt64)
	assert.Equal(t, int64(10), s.available())
}

// AC-15: greedy refill is exact — stepwise refills equal a single one-shot refill
// for the same elapsed time (the rounding-error carry prevents drift).
func TestAC15_GreedyRefillExactNoDrift(t *testing.T) {
	bw := NewBandwidth(1_000_000_000).RefillGreedy(3, time.Second).WithInitialTokens(0)

	for _, total := range []int64{sec, 2 * sec, 5 * sec, 7*sec + sec/3} {
		oneShot := newState(bw, 0)
		oneShot.refill(total)

		stepwise := newState(bw, 0)
		const steps = 37
		for i := int64(1); i <= steps; i++ {
			stepwise.refill(total * i / steps)
		}
		assert.Equal(t, oneShot.available(), stepwise.available(),
			"stepwise must match one-shot for elapsed=%d", total)
	}
}

// AC-16: initial tokens are honored at construction.
func TestAC16_InitialTokens(t *testing.T) {
	s := newState(NewBandwidth(10).RefillGreedy(1, time.Second).WithInitialTokens(3), 0)
	assert.Equal(t, int64(3), s.available())
}

// AC-17 (wait calc): delay to consume n = deficit * period / refillTokens (greedy).
func TestAC17_DelayNanosForConsume(t *testing.T) {
	s := newState(NewBandwidth(100).RefillGreedy(10, time.Second).WithInitialTokens(0), 0)

	assert.Equal(t, int64(0), s.delayNanosForConsume(0, 0))
	assert.Equal(t, 500*int64(time.Millisecond), s.delayNanosForConsume(5, 0)) // 5 tokens @10/s
	// n beyond capacity can never be satisfied.
	assert.Equal(t, int64(math.MaxInt64), s.delayNanosForConsume(101, 0))
}

// AC-17 (wait calc, intervally): delay to consume n only lands on whole-period
// boundaries — a fractional wait still rounds up to the next boundary.
func TestAC17_DelayNanosForConsumeIntervally(t *testing.T) {
	bw := NewBandwidth(100).RefillIntervally(10, time.Second).WithInitialTokens(0)
	s := newState(bw, 0)

	// Within one refill's worth: wait for the very next boundary.
	assert.Equal(t, sec, s.delayNanosForConsume(5, 0))
	// A "now" partway through the period shortens the wait accordingly.
	assert.Equal(t, 7*sec/10, s.delayNanosForConsume(5, 3*sec/10))
	// More than one refill's worth but less than two: one extra whole period.
	assert.Equal(t, 2*sec, s.delayNanosForConsume(15, 0))
	// Several periods needed, rounded up to a whole boundary count.
	assert.Equal(t, 3*sec, s.delayNanosForConsume(25, 0))
	assert.Equal(t, 3*sec, s.delayNanosForConsume(30, 0))
	// n beyond capacity can never be satisfied.
	assert.Equal(t, int64(math.MaxInt64), s.delayNanosForConsume(101, 0))
}

// AC-14/17: fullRefillNanos is the time to top up to capacity.
func TestFullRefillNanos(t *testing.T) {
	s := newState(NewBandwidth(100).RefillGreedy(10, time.Second).WithInitialTokens(0), 0)
	require.Equal(t, int64(0), s.available())
	assert.Equal(t, 10*sec, s.fullRefillNanos(0)) // 100 tokens @10/s = 10s
	s.currentSize = 100
	assert.Equal(t, int64(0), s.fullRefillNanos(0))
}

func TestMulOrMax(t *testing.T) {
	assert.Equal(t, int64(6), mulOrMax(2, 3))
	assert.Equal(t, int64(0), mulOrMax(0, math.MaxInt64))
	assert.Equal(t, int64(math.MaxInt64), mulOrMax(math.MaxInt64, 2))
}
