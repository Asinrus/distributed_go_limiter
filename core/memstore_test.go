package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC-45: expire-after-write — stored state disappears after the TTL, so the bucket
// reads as fresh again.
func TestAC45_InMemoryExpiration(t *testing.T) {
	clk := newFakeClock()
	pm := NewInMemoryProxyManager(WithClock(clk.now), WithExpireAfterWrite(10*time.Minute))
	ctx := context.Background()
	cfg := NewConfig(NewBandwidth(10).RefillGreedy(1, time.Hour))
	key := "expiring"

	b, err := pm.Bucket(key, cfg) // stateless proxy — state lives in the store
	require.NoError(t, err)

	ok, err := b.TryConsume(ctx, 4)
	require.NoError(t, err)
	require.True(t, ok)

	av, err := b.AvailableTokens(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(6), av, "state persisted before TTL")

	clk.advance(11 * time.Minute)

	av, err = b.AvailableTokens(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(10), av, "state expired -> fresh full bucket")
}

// A CAS create-if-absent race: only one create wins.
func TestMemStoreCreateIfAbsent(t *testing.T) {
	s := NewMemStore()
	ctx := context.Background()

	swapped, err := s.CompareAndSwap(ctx, "k", nil, []byte("v1"), 0)
	require.NoError(t, err)
	assert.True(t, swapped, "first create wins")

	swapped, err = s.CompareAndSwap(ctx, "k", nil, []byte("v2"), 0)
	require.NoError(t, err)
	assert.False(t, swapped, "second create loses")

	// Stale CAS (wrong old) fails; correct old succeeds.
	swapped, _ = s.CompareAndSwap(ctx, "k", []byte("wrong"), []byte("v3"), 0)
	assert.False(t, swapped)
	swapped, _ = s.CompareAndSwap(ctx, "k", []byte("v1"), []byte("v3"), 0)
	assert.True(t, swapped)
}
