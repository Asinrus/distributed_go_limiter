package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC-49: WithInitialTokens outside [0, capacity] is rejected at config
// validation time, through both constructors that share Config.valid().
func TestAC49_InitialTokensOverCapacityRejected(t *testing.T) {
	cfg := NewConfig(NewBandwidth(5).RefillGreedy(1, time.Second).WithInitialTokens(50))

	_, err := NewLocalBucket(cfg)
	assert.ErrorIs(t, err, ErrInvalidConfig)

	_, err = NewInMemoryProxyManager().Bucket("k", cfg)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

func TestAC49_NegativeInitialTokensRejected(t *testing.T) {
	cfg := NewConfig(NewBandwidth(5).RefillGreedy(1, time.Second).WithInitialTokens(-1))

	_, err := NewLocalBucket(cfg)
	assert.ErrorIs(t, err, ErrInvalidConfig)

	_, err = NewInMemoryProxyManager().Bucket("k", cfg)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

func TestAC49_InitialTokensAtCapacityAccepted(t *testing.T) {
	cfg := NewConfig(NewBandwidth(5).RefillGreedy(1, time.Second))

	b, err := NewLocalBucket(cfg)
	require.NoError(t, err)
	require.NotNil(t, b)
}

func TestAC49_InitialTokensZeroAccepted(t *testing.T) {
	cfg := NewConfig(NewBandwidth(5).RefillGreedy(1, time.Second).WithInitialTokens(0))

	b, err := NewLocalBucket(cfg)
	require.NoError(t, err)
	require.NotNil(t, b)

	ctx := t.Context()
	ok, err := b.TryConsume(ctx, 1)
	require.NoError(t, err)
	assert.False(t, ok, "empty bucket must not serve a token immediately")
}

// A valid smaller initialTokens still refills up to capacity and no further:
// the guard changed rejection only, not the refill math.
func TestAC49_SmallerInitialTokensStillRefillsToCapacity(t *testing.T) {
	clk := newFakeClock()
	cfg := NewConfig(NewBandwidth(5).RefillGreedy(1, time.Second).WithInitialTokens(2))
	b := localWithClock(cfg, clk)
	ctx := t.Context()

	clk.advance(10 * time.Second) // far more than enough to refill to capacity

	available, err := b.AvailableTokens(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(5), available, "must cap at capacity, not overflow")
}
