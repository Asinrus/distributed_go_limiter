package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodecRoundTrip(t *testing.T) {
	bw := NewBandwidth(100).RefillGreedy(10, time.Second)
	s := &bucketState{lastRefillNanos: 123456789, currentSize: 45, roundingError: 6, bw: bw}

	data := encodeState(s)
	require.Len(t, data, stateByteSize)

	got, err := decodeState(data, bw)
	require.NoError(t, err)
	assert.Equal(t, int64(123456789), got.lastRefillNanos)
	assert.Equal(t, int64(45), got.currentSize)
	assert.Equal(t, int64(6), got.roundingError)

	// Negative values (reservations) survive the round trip.
	s.currentSize = -7
	got, err = decodeState(encodeState(s), bw)
	require.NoError(t, err)
	assert.Equal(t, int64(-7), got.currentSize)
}

func TestCodecRejectsBadInput(t *testing.T) {
	bw := NewBandwidth(100).RefillGreedy(10, time.Second)
	data := encodeState(&bucketState{bw: bw})

	_, err := decodeState(data[:10], bw)
	assert.Error(t, err, "short buffer")

	bad := append([]byte(nil), data...)
	bad[0] = 99
	_, err = decodeState(bad, bw)
	assert.Error(t, err, "wrong version")
}
