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

// Store failures must be distinguishable from misuse ("wrong API call") and from a
// plain refusal ("bucket said no"), so a caller can pick its own degradation.
// No acceptance criterion covers this yet — see the report to the Architect.

// backendErr is a concrete backend error type, so we can prove the original stays
// reachable through errors.As after core wraps it.
type backendErr struct{ op string }

func (e *backendErr) Error() string { return "fake backend: " + e.op + " failed" }

var errDown = &backendErr{op: "dial"}

// stubCAS is a MemStore with injectable failures at each store boundary.
type stubCAS struct {
	inner      *core.MemStore
	getErr     error
	casErr     error
	rmErr      error
	neverSwaps bool // always lose the CAS race -> retries exhausted
}

func newStubCAS() *stubCAS { return &stubCAS{inner: core.NewMemStore()} }

func (s *stubCAS) GetState(ctx context.Context, key string) ([]byte, bool, error) {
	if s.getErr != nil {
		return nil, false, s.getErr
	}
	return s.inner.GetState(ctx, key)
}

func (s *stubCAS) CompareAndSwap(ctx context.Context, key string, old, newState []byte, ttl time.Duration) (bool, error) {
	if s.casErr != nil {
		return false, s.casErr
	}
	if s.neverSwaps {
		return false, nil
	}
	return s.inner.CompareAndSwap(ctx, key, old, newState, ttl)
}

func (s *stubCAS) Remove(ctx context.Context, key string) error {
	if s.rmErr != nil {
		return s.rmErr
	}
	return s.inner.Remove(ctx, key)
}

// stubLock is a LockStore that either fails outright or hands fn the bytes it holds
// (including garbage) and propagates fn's own error, exactly like a real one.
type stubLock struct {
	doErr error
	rmErr error
	state []byte
	found bool
}

func (s *stubLock) Do(_ context.Context, _ string, _ time.Duration,
	fn func(old []byte, found bool) ([]byte, bool, error)) error {
	if s.doErr != nil {
		return s.doErr
	}
	newState, write, err := fn(s.state, s.found)
	if err != nil {
		return err // the callback's error travels back out through Do
	}
	if write {
		s.state, s.found = newState, true
	}
	return nil
}

func (s *stubLock) Remove(context.Context, string) error { return s.rmErr }

func testConfig() core.Config {
	return core.NewConfig(core.NewBandwidth(10).RefillGreedy(1, time.Second))
}

func TestStoreErr_GetStateFailureIsStoreError(t *testing.T) {
	s := newStubCAS()
	s.getErr = errDown
	b, err := core.NewCASProxyManager(s).Bucket("k", testConfig())
	require.NoError(t, err)

	_, err = b.TryConsume(context.Background(), 1)
	assert.ErrorIs(t, err, core.ErrStore)
}

func TestStoreErr_CompareAndSwapFailureIsStoreError(t *testing.T) {
	s := newStubCAS()
	s.casErr = errDown
	b, err := core.NewCASProxyManager(s).Bucket("k", testConfig())
	require.NoError(t, err)

	_, err = b.TryConsume(context.Background(), 1)
	assert.ErrorIs(t, err, core.ErrStore)
}

func TestStoreErr_RemoveFailureIsStoreError(t *testing.T) {
	cas := newStubCAS()
	cas.rmErr = errDown
	assert.ErrorIs(t, core.NewCASProxyManager(cas).Remove(context.Background(), "k"), core.ErrStore)

	lock := &stubLock{rmErr: errDown}
	assert.ErrorIs(t, core.NewLockProxyManager(lock).Remove(context.Background(), "k"), core.ErrStore)
}

func TestStoreErr_LockStoreDoFailureIsStoreError(t *testing.T) {
	b, err := core.NewLockProxyManager(&stubLock{doErr: errDown}).Bucket("k", testConfig())
	require.NoError(t, err)

	_, err = b.TryConsume(context.Background(), 1)
	assert.ErrorIs(t, err, core.ErrStore)
}

// Retries exhausted is a failure of the store layer to complete the operation, so a
// caller degrading on ErrStore must catch it too — while still being able to tell it
// apart from an outage.
func TestStoreErr_CASRetriesExhaustedIsStoreError(t *testing.T) {
	s := newStubCAS()
	s.neverSwaps = true
	b, err := core.NewCASProxyManager(s).Bucket("k", testConfig())
	require.NoError(t, err)

	_, err = b.TryConsume(context.Background(), 1)
	assert.ErrorIs(t, err, core.ErrCASRetriesExhausted)
	assert.ErrorIs(t, err, core.ErrStore)
}

// The trap: a decode failure originates in core's own callback and only travels back
// through the store's return value. It must NOT be reported as a store failure.
func TestStoreErr_CorruptStateIsNotStoreError(t *testing.T) {
	ctx := context.Background()
	garbage := []byte("not a bucket state")

	t.Run("lock path", func(t *testing.T) {
		b, err := core.NewLockProxyManager(&stubLock{state: garbage, found: true}).Bucket("k", testConfig())
		require.NoError(t, err)

		_, err = b.TryConsume(ctx, 1)
		require.Error(t, err)
		assert.NotErrorIs(t, err, core.ErrStore, "corrupt state is a decode failure, not a store failure")
	})

	t.Run("cas path", func(t *testing.T) {
		s := newStubCAS()
		_, err := s.CompareAndSwap(ctx, "k", nil, garbage, 0)
		require.NoError(t, err)
		b, err := core.NewCASProxyManager(s).Bucket("k", testConfig())
		require.NoError(t, err)

		_, err = b.TryConsume(ctx, 1)
		require.Error(t, err)
		assert.NotErrorIs(t, err, core.ErrStore)
	})
}

// Misuse and refusal stay clear of ErrStore.
func TestStoreErr_MisuseAndRefusalAreNotStoreErrors(t *testing.T) {
	ctx := context.Background()
	b, err := core.NewInMemoryProxyManager().Bucket("k", testConfig())
	require.NoError(t, err)

	_, err = b.TryConsume(ctx, 0)
	assert.ErrorIs(t, err, core.ErrNonPositiveTokens)
	assert.NotErrorIs(t, err, core.ErrStore)

	ok, err := b.TryConsume(ctx, 11) // more than capacity: refused, not an error
	require.NoError(t, err)
	assert.False(t, ok)
}

// Wrapping must not hide the backend's own error from the caller.
func TestStoreErr_OriginalBackendErrorStaysReachable(t *testing.T) {
	s := newStubCAS()
	s.getErr = errDown
	b, err := core.NewCASProxyManager(s).Bucket("k", testConfig())
	require.NoError(t, err)

	_, err = b.TryConsume(context.Background(), 1)
	require.ErrorIs(t, err, core.ErrStore)
	assert.ErrorIs(t, err, errDown)

	var be *backendErr
	require.True(t, errors.As(err, &be))
	assert.Equal(t, "dial", be.op)
}
