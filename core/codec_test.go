package core

import (
	"encoding/hex"
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

// The wire format is persisted in Redis/Postgres and outlives the process that
// wrote it, so the byte layout is a contract — with future versions of this code,
// and with anything else that reads the blob (a psql session, a Lua script, an
// ops tool).
//
// TestCodecRoundTrip cannot protect that contract: encode and decode share the
// same offsets, so transposing a pair in BOTH still round-trips perfectly while
// every stored byte moves and every existing bucket silently decodes as garbage.
// This pins the layout to a literal instead, in both directions.
//
// If this test fails, the format changed: bump stateVersion and handle the old
// one in decodeState — do not just update the literal.
func TestCodecWireFormatIsFrozen(t *testing.T) {
	bw := NewBandwidth(100).RefillGreedy(10, time.Second)

	// Distinct bit pattern per field, so a transposition is obvious by eye.
	s := &bucketState{
		lastRefillNanos: 0x0102030405060708,
		currentSize:     0x1112131415161718,
		roundingError:   0x2122232425262728,
		bw:              bw,
	}

	want := []byte{
		0x01,                                           // [0]     version
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, // [1:9]   lastRefillNanos, big-endian
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, // [9:17]  currentSize
		0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, // [17:25] roundingError
	}

	// Hex, so a failure shows exactly which byte moved.
	assert.Equal(t, hex.EncodeToString(want), hex.EncodeToString(encodeState(s)),
		"encoded layout changed")

	// Decode the hand-written literal too: this half guards the reader against a
	// drift the writer shares, and doubles as the spec for non-Go decoders.
	got, err := decodeState(want, bw)
	require.NoError(t, err)
	assert.Equal(t, int64(0x0102030405060708), got.lastRefillNanos, "lastRefillNanos offset")
	assert.Equal(t, int64(0x1112131415161718), got.currentSize, "currentSize offset")
	assert.Equal(t, int64(0x2122232425262728), got.roundingError, "roundingError offset")
}

// Compare-and-swap compares stored bytes for equality, so encoding MUST be
// deterministic — the same state has to produce the same bytes every time. A
// codec with map iteration or a self-describing stream format (gob emits a type
// descriptor only on a fresh encoder) would break every CAS instead of failing
// loudly: writes would just never land, and the retry loop would exhaust.
func TestCodecIsDeterministic(t *testing.T) {
	bw := NewBandwidth(100).RefillGreedy(10, time.Second)
	s := &bucketState{lastRefillNanos: 1761000000123456789, currentSize: 42, roundingError: 987654321, bw: bw}

	first := encodeState(s)
	for i := 0; i < 3; i++ {
		assert.Equal(t, first, encodeState(s), "encode %d differed", i)
	}
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
