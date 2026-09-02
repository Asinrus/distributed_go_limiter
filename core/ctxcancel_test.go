package core_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC-50: a cancelled or expired context must never let a distributed op consume a
// token, and the error it produces must be distinguishable from a genuine store
// failure. The decision: the context error is wrapped in core.ErrStore (bucket.go
// already documented ErrStore as covering cancellation), but errors.Is against the
// concrete context error keeps it distinguishable from an unrelated backend outage.

func TestAC50_CancelledContextRejectsTryConsume(t *testing.T) {
	pm := core.NewInMemoryProxyManager()
	cfg := testConfig()
	b, err := pm.Bucket("k", cfg)
	require.NoError(t, err)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	ok, err := b.TryConsume(cancelled, 1)
	assert.False(t, ok)
	assert.ErrorIs(t, err, context.Canceled)
	assert.ErrorIs(t, err, core.ErrStore)

	av, err := b.AvailableTokens(context.Background())
	require.NoError(t, err)
	assert.Equal(t, cfg.Bandwidth().Capacity(), av, "cancelled call must not have spent a token")
}

func TestAC50_CancelledContextRejectsAllOps(t *testing.T) {
	pm := core.NewInMemoryProxyManager()
	b, err := pm.Bucket("k", testConfig())
	require.NoError(t, err)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("TryConsumeAndReturnRemaining", func(t *testing.T) {
		_, err := b.TryConsumeAndReturnRemaining(cancelled, 1)
		assert.ErrorIs(t, err, context.Canceled)
	})
	t.Run("EstimateAbilityToConsume", func(t *testing.T) {
		_, err := b.EstimateAbilityToConsume(cancelled, 1)
		assert.ErrorIs(t, err, context.Canceled)
	})
	t.Run("TryConsumeAsMuchAsPossible", func(t *testing.T) {
		_, err := b.TryConsumeAsMuchAsPossible(cancelled, 0)
		assert.ErrorIs(t, err, context.Canceled)
	})

	av, err := b.AvailableTokens(context.Background())
	require.NoError(t, err)
	assert.Equal(t, testConfig().Bandwidth().Capacity(), av, "no read-only op should have consumed a token")
}

func TestAC50_ContextErrorDistinguishableFromStoreFailure(t *testing.T) {
	pm := core.NewInMemoryProxyManager()
	b, err := pm.Bucket("k", testConfig())
	require.NoError(t, err)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, cancelErr := b.TryConsume(cancelled, 1)
	assert.ErrorIs(t, cancelErr, context.Canceled)
	assert.NotErrorIs(t, cancelErr, errDown)

	s := newStubCAS()
	s.getErr = errDown
	b2, err := core.NewCASProxyManager(s).Bucket("k", testConfig())
	require.NoError(t, err)
	_, storeErr := b2.TryConsume(context.Background(), 1)
	assert.ErrorIs(t, storeErr, core.ErrStore)
	assert.NotErrorIs(t, storeErr, context.Canceled)
}

func TestAC50_ExpiredDeadlineBehavesLikeCancellation(t *testing.T) {
	pm := core.NewInMemoryProxyManager()
	b, err := pm.Bucket("k", testConfig())
	require.NoError(t, err)

	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	ok, err := b.TryConsume(expired, 1)
	assert.False(t, ok)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	av, err := b.AvailableTokens(context.Background())
	require.NoError(t, err)
	assert.Equal(t, testConfig().Bandwidth().Capacity(), av)
}

func TestAC50_MemStoreReturnsContextErrorDirectly(t *testing.T) {
	s := core.NewMemStore()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := s.GetState(cancelled, "k")
	assert.Equal(t, context.Canceled, err)

	_, err = s.CompareAndSwap(cancelled, "k", nil, []byte("v"), 0)
	assert.Equal(t, context.Canceled, err)

	err = s.Remove(cancelled, "k")
	assert.Equal(t, context.Canceled, err)
}

// cancelAfterNGetState cancels the context after a fixed number of GetState calls,
// simulating cancellation arriving mid-retry-loop.
type cancelAfterNGetState struct {
	inner  *core.MemStore
	cancel context.CancelFunc
	after  int
	calls  int
}

func (s *cancelAfterNGetState) GetState(ctx context.Context, key string) ([]byte, bool, error) {
	s.calls++
	if s.calls > s.after {
		s.cancel()
	}
	return s.inner.GetState(ctx, key)
}

func (s *cancelAfterNGetState) CompareAndSwap(ctx context.Context, key string, old, newState []byte, ttl time.Duration) (bool, error) {
	return false, nil // never swaps: forces the retry loop to keep going
}

func (s *cancelAfterNGetState) Remove(ctx context.Context, key string) error {
	return s.inner.Remove(ctx, key)
}

func TestAC50_CancellationMidRetryStopsLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := &cancelAfterNGetState{inner: core.NewMemStore(), cancel: cancel, after: 2}

	b, err := core.NewCASProxyManager(store).Bucket("k", testConfig())
	require.NoError(t, err)

	_, err = b.TryConsume(ctx, 1)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled, "should stop on cancellation, not exhaust retries")
	assert.False(t, errors.Is(err, core.ErrCASRetriesExhausted), "must not run out the remaining attempts")
	assert.LessOrEqual(t, store.calls, 4, "loop should stop shortly after cancellation, not spin to maxCASRetries")
}
