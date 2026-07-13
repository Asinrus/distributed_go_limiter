package core

import (
	"encoding/binary"
	"fmt"
)

// State wire format: 1 version byte + 3 big-endian int64s (25 bytes total).
// Only the three mutable state values are persisted; the Bandwidth comes from the
// client config. Encoding is deterministic so CAS byte-equality is meaningful.
// Go owns both sides of the wire, so no bucket4j binary compatibility is attempted.
const (
	stateVersion  = 1
	stateByteSize = 1 + 3*8
)

func encodeState(s *bucketState) []byte {
	buf := make([]byte, stateByteSize)
	buf[0] = stateVersion
	binary.BigEndian.PutUint64(buf[1:], uint64(s.lastRefillNanos))
	binary.BigEndian.PutUint64(buf[9:], uint64(s.currentSize))
	binary.BigEndian.PutUint64(buf[17:], uint64(s.roundingError))
	return buf
}

func decodeState(data []byte, bw Bandwidth) (*bucketState, error) {
	if len(data) != stateByteSize {
		return nil, fmt.Errorf("core: bad state length %d, want %d", len(data), stateByteSize)
	}
	if data[0] != stateVersion {
		return nil, fmt.Errorf("core: unsupported state version %d", data[0])
	}
	return &bucketState{
		lastRefillNanos: int64(binary.BigEndian.Uint64(data[1:])),
		currentSize:     int64(binary.BigEndian.Uint64(data[9:])),
		roundingError:   int64(binary.BigEndian.Uint64(data[17:])),
		bw:              bw,
	}, nil
}
