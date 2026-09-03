package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC-50: a cancelled or expired context must never let a distributed op consume a
// token, and the error the caller sees is the bare context error — not ErrStore.
// ErrStore is the fail-open/fail-closed switch for a backend outage, and a client
// that hung up is not an outage (docs/adr/0015-cancellation-is-not-a-store-failure.md).

// ctxIgnoringCAS never looks at its context, so every AC-50 assertion below proves
// the executor's guard rather than a diligent store. Driving MemStore here would
// pass even with the guard deleted.
type ctxIgnoringCAS struct {
	inner      *core.MemStore
	getErr     error
	neverSwaps bool
	onGet      func()
	gets       int
}

func newCtxIgnoringCAS() *ctxIgnoringCAS { return &ctxIgnoringCAS{inner: core.NewMemStore()} }

func (s *ctxIgnoringCAS) GetState(_ context.Context, key string) ([]byte, bool, error) {
	s.gets++
	if s.onGet != nil {
		s.onGet()
	}
	if s.getErr != nil {
		return nil, false, s.getErr
	}
	return s.inner.GetState(context.Background(), key)
}

func (s *ctxIgnoringCAS) CompareAndSwap(_ context.Context, key string, old, newState []byte, ttl time.Duration) (bool, error) {
	if s.neverSwaps {
		return false, nil
	}
	return s.inner.CompareAndSwap(context.Background(), key, old, newState, ttl)
}

func (s *ctxIgnoringCAS) Remove(_ context.Context, key string) error {
	return s.inner.Remove(context.Background(), key)
}

func cancelledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestAC50_CancelledContextRejectsTryConsume(t *testing.T) {
	store := newCtxIgnoringCAS()
	b, err := core.NewCASProxyManager(store).Bucket("k", testConfig())
	require.NoError(t, err)

	ok, err := b.TryConsume(cancelledCtx(), 1)
	assert.False(t, ok)
	assert.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, core.ErrStore, "a cancelled client is not a store outage")
	assert.Zero(t, store.gets, "no work should reach the store at all")

	av, err := b.AvailableTokens(context.Background())
	require.NoError(t, err)
	assert.Equal(t, testConfig().Bandwidth().Capacity(), av, "cancelled call must not have spent a token")
}

func TestAC50_CancelledContextRejectsAllOps(t *testing.T) {
	b, err := core.NewCASProxyManager(newCtxIgnoringCAS()).Bucket("k", testConfig())
	require.NoError(t, err)
	ctx := cancelledCtx()

	t.Run("TryConsumeAndReturnRemaining", func(t *testing.T) {
		_, err := b.TryConsumeAndReturnRemaining(ctx, 1)
		assert.ErrorIs(t, err, context.Canceled)
		assert.NotErrorIs(t, err, core.ErrStore)
	})
	t.Run("EstimateAbilityToConsume", func(t *testing.T) {
		_, err := b.EstimateAbilityToConsume(ctx, 1)
		assert.ErrorIs(t, err, context.Canceled)
		assert.NotErrorIs(t, err, core.ErrStore)
	})
	t.Run("TryConsumeAsMuchAsPossible", func(t *testing.T) {
		_, err := b.TryConsumeAsMuchAsPossible(ctx, 0)
		assert.ErrorIs(t, err, context.Canceled)
		assert.NotErrorIs(t, err, core.ErrStore)
	})
	t.Run("AvailableTokens", func(t *testing.T) {
		_, err := b.AvailableTokens(ctx)
		assert.ErrorIs(t, err, context.Canceled)
		assert.NotErrorIs(t, err, core.ErrStore)
	})

	av, err := b.AvailableTokens(context.Background())
	require.NoError(t, err)
	assert.Equal(t, testConfig().Bandwidth().Capacity(), av, "no read-only op should have consumed a token")
}

func TestAC50_LockPathRejectsCancelledContext(t *testing.T) {
	b, err := core.NewLockProxyManager(&stubLock{}).Bucket("k", testConfig())
	require.NoError(t, err)

	ok, err := b.TryConsume(cancelledCtx(), 1)
	assert.False(t, ok)
	assert.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, core.ErrStore)

	av, err := b.AvailableTokens(context.Background())
	require.NoError(t, err)
	assert.Equal(t, testConfig().Bandwidth().Capacity(), av)
}

func TestAC50_ExpiredDeadlineBehavesLikeCancellation(t *testing.T) {
	cas, err := core.NewCASProxyManager(newCtxIgnoringCAS()).Bucket("k", testConfig())
	require.NoError(t, err)
	lock, err := core.NewLockProxyManager(&stubLock{}).Bucket("k", testConfig())
	require.NoError(t, err)

	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	for name, bucket := range map[string]core.Bucket{"cas": cas, "lock": lock} {
		t.Run(name, func(t *testing.T) {
			ok, err := bucket.TryConsume(expired, 1)
			assert.False(t, ok)
			assert.ErrorIs(t, err, context.DeadlineExceeded)
			assert.NotErrorIs(t, err, core.ErrStore)

			av, err := bucket.AvailableTokens(context.Background())
			require.NoError(t, err)
			assert.Equal(t, testConfig().Bandwidth().Capacity(), av)
		})
	}
}

func TestAC50_CancellationMidRetryStopsLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := newCtxIgnoringCAS()
	store.neverSwaps = true // every attempt loses the race, so only ctx can stop the loop
	store.onGet = func() {
		if store.gets > 2 {
			cancel()
		}
	}

	b, err := core.NewCASProxyManager(store).Bucket("k", testConfig())
	require.NoError(t, err)

	_, err = b.TryConsume(ctx, 1)
	assert.ErrorIs(t, err, context.Canceled, "should stop on cancellation, not exhaust retries")
	assert.NotErrorIs(t, err, core.ErrCASRetriesExhausted, "must not run out the remaining attempts")
	assert.NotErrorIs(t, err, core.ErrStore)
	assert.LessOrEqual(t, store.gets, 4, "loop should stop shortly after cancellation")
}

// The choke point made visible: storeErr reports the cancellation and drops the
// backend's own error. Deliberate — that error is almost always ctx.Err() again.
func TestAC50_StoreErrorDuringCancellationReportsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := newCtxIgnoringCAS()
	store.getErr = errDown
	store.onGet = cancel // the round trip fails and the context ends together

	b, err := core.NewCASProxyManager(store).Bucket("k", testConfig())
	require.NoError(t, err)

	_, err = b.TryConsume(ctx, 1)
	assert.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, core.ErrStore)
	assert.NotErrorIs(t, err, errDown)
}

// A live context keeps the ErrStore shape intact in both directions — the boundary
// this change moves, pinned from the AC-50 side.
func TestAC50_StoreFailureOnLiveContextStaysStoreError(t *testing.T) {
	store := newCtxIgnoringCAS()
	store.getErr = errDown
	b, err := core.NewCASProxyManager(store).Bucket("k", testConfig())
	require.NoError(t, err)

	_, err = b.TryConsume(context.Background(), 1)
	assert.ErrorIs(t, err, core.ErrStore)
	assert.ErrorIs(t, err, errDown)
	assert.NotErrorIs(t, err, context.Canceled)
}

// Hygiene only, never reached through a Bucket: the reference CASStore honours its
// context, so a backend author copying it does not learn to discard one.
func TestAC50_MemStoreReturnsContextErrorDirectly(t *testing.T) {
	s := core.NewMemStore()
	ctx := cancelledCtx()

	_, _, err := s.GetState(ctx, "k")
	assert.Equal(t, context.Canceled, err)

	_, err = s.CompareAndSwap(ctx, "k", nil, []byte("v"), 0)
	assert.Equal(t, context.Canceled, err)

	assert.Equal(t, context.Canceled, s.Remove(ctx, "k"))
}
