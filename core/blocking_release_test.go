package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC-54: a cancelled blocking Consume refunds the tokens it reserved.
// ai-context/tasks/core-blocking-reservation-refund.md.

// AC-53 and AC-54 together: a bucket drained to empty, a blocking Consume
// cancelled mid-wait, and AvailableTokens read before, during, and after. The
// middle reading is the clamp (AC-53); the last is the refund (AC-54) — run
// together they also prove the clamp is not silently doing the refund's job.
func TestAC53AC54_ClampAndRefundTogether(t *testing.T) {
	clk := newFakeClock()
	// 10 tokens/s, empty: 5 tokens need 500ms.
	b := localWithClock(NewConfig(NewBandwidth(10).RefillGreedy(10, time.Second).WithInitialTokens(0)), clk)
	ctx := context.Background()

	before, err := b.AvailableTokens(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), before)

	cctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- b.AsBlocking().Consume(cctx, 5)
	}()

	time.Sleep(50 * time.Millisecond) // reservation is made and the wait is underway
	during, err := b.AvailableTokens(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), during, "AC-53: the clamp hides the negative ledger during the reservation")

	cancel()
	consumeErr := <-done
	assert.ErrorIs(t, consumeErr, context.Canceled)
	assert.NotErrorIs(t, consumeErr, ErrReservationNotReleased)

	after, err := b.AvailableTokens(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before, after, "AC-54: the refund restores the pre-reservation count")
}

// The distributed path proves the release travels through the executor and
// codec, not only through LocalBucket's in-memory state.
func TestAC54_DistributedCancelledConsumeRefunds(t *testing.T) {
	clk := newFakeClock()
	pm := NewInMemoryProxyManager(WithClock(clk.now))
	b, err := pm.Bucket("k", NewConfig(NewBandwidth(10).RefillGreedy(10, time.Second).WithInitialTokens(0)))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err = b.AsBlocking().Consume(ctx, 5)
	require.ErrorIs(t, err, context.Canceled)

	avail, err := b.AvailableTokens(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), avail, "reservation refunded through the store")
}

// A deadline that expires mid-wait behaves the same as an explicit cancel.
func TestAC54_DeadlineExceededMidWaitAlsoReleases(t *testing.T) {
	clk := newFakeClock()
	// Delay the store's GetState so most of the ctx budget is spent inside
	// reserveTokens itself; by the time the real wait timer starts, less of
	// the deadline remains than the timer's duration, so ctx.Done() wins the
	// select deterministically rather than by coincidence.
	store := &slowFirstGetCAS{inner: newMemStoreClock(clk.now), delay: 500 * time.Millisecond}
	pm := NewCASProxyManager(store, WithClock(clk.now))
	b, err := pm.Bucket("k", NewConfig(NewBandwidth(1000).RefillGreedy(1000, time.Second).WithInitialTokens(0)))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 650*time.Millisecond)
	defer cancel()

	// 300 tokens @ 1000/s -> 300ms wait, granted against the ~650ms budget.
	err = b.AsBlocking().Consume(ctx, 300)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	avail, err := b.AvailableTokens(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), avail, "reservation refunded after a deadline mid-wait")
}

// slowFirstGetCAS delays its first GetState call, simulating a slow round trip
// that eats into the caller's remaining deadline between reservation and wait.
type slowFirstGetCAS struct {
	inner    *MemStore
	delay    time.Duration
	getCalls int
}

func (s *slowFirstGetCAS) GetState(ctx context.Context, key string) ([]byte, bool, error) {
	s.getCalls++
	if s.getCalls == 1 && s.delay > 0 {
		time.Sleep(s.delay)
	}
	return s.inner.GetState(ctx, key)
}

func (s *slowFirstGetCAS) CompareAndSwap(ctx context.Context, key string, old, newState []byte, ttl time.Duration) (bool, error) {
	return s.inner.CompareAndSwap(ctx, key, old, newState, ttl)
}

func (s *slowFirstGetCAS) Remove(ctx context.Context, key string) error {
	return s.inner.Remove(ctx, key)
}

// The ordinary case: a wait that completes well before cancellation returns nil
// via the t.C branch of select, before ctx.Done() is even reached. This does
// NOT exercise the double-spend guard (see the reservationOutcome tests below
// for that) — Go schedules the goroutine off the timer's own readiness, so by
// the time cancel() runs the wait is already over and the select has already
// returned. It is here only to pin the unremarkable end-to-end behaviour.
func TestAC54_ConsumeSucceedsWhenWaitCompletesBeforeCancel(t *testing.T) {
	clk := newFakeClock()
	b := localWithClock(NewConfig(NewBandwidth(10).RefillGreedy(1000, time.Second).WithInitialTokens(0)), clk)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond) // long past the 5ms wait
		cancel()
	}()

	err := b.AsBlocking().Consume(ctx, 5)
	assert.NoError(t, err)

	avail, err := b.AvailableTokens(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), avail, "tokens stay consumed exactly once, not released")
}

// TestAC54_ReservationOutcomeGuardsAgainstDoubleSpend is the actual coverage for
// the double-spend guard at core/blocking.go's ctx.Done() branch. select's
// choice between two simultaneously-ready channels is arbitrary at runtime — no
// test can force that choice without probabilistic looping — so the decision it
// gates is pulled out into reservationOutcome, a pure function with no select in
// it. Calling it directly with timerElapsed=true is the deterministic
// equivalent of "the timer had already fired when select happened to wake on
// ctx.Done() instead": release must not run, and nil must come back.
func TestAC54_ReservationOutcomeGuardsAgainstDoubleSpend(t *testing.T) {
	released := false
	err := reservationOutcome(true, context.Canceled, func() error {
		released = true
		return nil
	})
	assert.NoError(t, err, "the wait had already elapsed; the consumption must stand")
	assert.False(t, released, "must not double-spend by releasing an already-completed consumption")
}

// The pending case: the timer had not fired, so the release must run and the
// bare ctx error comes back.
func TestAC54_ReservationOutcomeReleasesWhenTimerStillPending(t *testing.T) {
	released := false
	err := reservationOutcome(false, context.Canceled, func() error {
		released = true
		return nil
	})
	assert.ErrorIs(t, err, context.Canceled)
	assert.True(t, released)
}

// A release failure folds ErrReservationNotReleased into the ctx error without
// replacing it.
func TestAC54_ReservationOutcomeReleaseFailureWraps(t *testing.T) {
	err := reservationOutcome(false, context.Canceled, func() error {
		return errFlakyStoreDown
	})
	assert.ErrorIs(t, err, context.Canceled)
	assert.ErrorIs(t, err, ErrReservationNotReleased)
}

// The release is clamped at capacity: refills that ran while the caller waited
// must not let the refund inflate the bucket above what it could ever hold.
// Driven on the injected clock, not wall time.
func TestAC54_ReleaseClampedAtCapacity(t *testing.T) {
	clk := newFakeClock()
	b := localWithClock(NewConfig(NewBandwidth(100).RefillGreedy(1000, time.Second).WithInitialTokens(0)), clk)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		clk.advance(time.Hour) // enough for refill to fully replenish the bucket
		cancel()
	}()

	err := b.AsBlocking().Consume(ctx, 50)
	require.ErrorIs(t, err, context.Canceled)

	avail, err := b.AvailableTokens(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(100), avail, "release must clamp at capacity, never above it")
}

// The reported failure: when the release itself fails, the error satisfies both
// errors.Is(err, context.Canceled) and errors.Is(err, ErrReservationNotReleased),
// and specifically not errors.Is(err, ErrStore) — dropping the store cause is
// deliberate (docs/adr/0015).
func TestAC54_ReleaseFailureReportsErrReservationNotReleased(t *testing.T) {
	clk := newFakeClock()
	store := &flakyReleaseCAS{inner: newMemStoreClock(clk.now), failFromCall: 2}
	pm := NewCASProxyManager(store, WithClock(clk.now))
	b, err := pm.Bucket("k", NewConfig(NewBandwidth(10).RefillGreedy(10, time.Second).WithInitialTokens(0)))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err = b.AsBlocking().Consume(ctx, 5)
	assert.ErrorIs(t, err, context.Canceled)
	assert.ErrorIs(t, err, ErrReservationNotReleased)
	assert.NotErrorIs(t, err, ErrStore)
}

// flakyReleaseCAS lets the first CompareAndSwap (the reservation) succeed and
// fails every one after it (the release).
type flakyReleaseCAS struct {
	inner        *MemStore
	failFromCall int
	casCalls     int
}

var errFlakyStoreDown = errors.New("flaky store: down")

func (s *flakyReleaseCAS) GetState(ctx context.Context, key string) ([]byte, bool, error) {
	return s.inner.GetState(ctx, key)
}

func (s *flakyReleaseCAS) CompareAndSwap(ctx context.Context, key string, old, newState []byte, ttl time.Duration) (bool, error) {
	s.casCalls++
	if s.failFromCall > 0 && s.casCalls >= s.failFromCall {
		return false, errFlakyStoreDown
	}
	return s.inner.CompareAndSwap(ctx, key, old, newState, ttl)
}

func (s *flakyReleaseCAS) Remove(ctx context.Context, key string) error {
	return s.inner.Remove(ctx, key)
}

// Unchanged behaviour: an uncancelled Consume still waits the same duration and
// still consumes exactly n. The existing AC-31/32 tests already cover this; this
// pins it against release specifically (no refund on a normal completion).
func TestAC54_UncancelledConsumeUnaffected(t *testing.T) {
	clk := newFakeClock()
	b := localWithClock(NewConfig(NewBandwidth(10).RefillGreedy(10, time.Second).WithInitialTokens(0)), clk)

	err := b.AsBlocking().Consume(context.Background(), 5)
	require.NoError(t, err)

	avail, err := b.AvailableTokens(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), avail, "no refund on a completed consume")
}
