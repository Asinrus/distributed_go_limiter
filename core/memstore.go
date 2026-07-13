package core

import (
	"bytes"
	"context"
	"sync"
	"time"
)

// MemStore is an in-memory CASStore: the reference provider implementation and the
// store the conformance suite runs against without a container. Safe for concurrent
// use. Exported so callers can run the distributed API in-process.
type MemStore struct {
	mu  sync.Mutex
	m   map[string]memEntry
	now func() int64 // for TTL expiry; matches the ProxyManager clock
}

type memEntry struct {
	data       []byte
	expiryNano int64 // 0 = never expires
}

// NewMemStore creates an empty in-memory store using a wall-clock TTL source.
func NewMemStore() *MemStore { return newMemStoreClock(wallNanos) }

func newMemStoreClock(now func() int64) *MemStore {
	return &MemStore{m: make(map[string]memEntry), now: now}
}

func (s *MemStore) liveEntry(key string) (memEntry, bool) {
	e, ok := s.m[key]
	if !ok {
		return memEntry{}, false
	}
	if e.expiryNano != 0 && s.now() >= e.expiryNano {
		delete(s.m, key)
		return memEntry{}, false
	}
	return e, true
}

func (s *MemStore) GetState(_ context.Context, key string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.liveEntry(key)
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), e.data...), true, nil
}

func (s *MemStore) CompareAndSwap(_ context.Context, key string, old, newState []byte, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, present := s.liveEntry(key)

	if old == nil {
		if present {
			return false, nil // someone created it first
		}
	} else {
		if !present || !bytes.Equal(cur.data, old) {
			return false, nil // expected value gone or changed
		}
	}

	var expiry int64
	if ttl > 0 {
		expiry = s.now() + ttl.Nanoseconds()
	}
	s.m[key] = memEntry{data: append([]byte(nil), newState...), expiryNano: expiry}
	return true, nil
}

func (s *MemStore) Remove(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

var _ CASStore = (*MemStore)(nil)

// NewInMemoryProxyManager builds a distributed-style ProxyManager backed by an
// in-memory MemStore. The store shares the manager's clock so TTLs behave under a
// test clock. Handy for tests and for running the distributed API without a container.
func NewInMemoryProxyManager(opts ...ProxyOption) *ProxyManager {
	c := newProxyConfig(opts...)
	store := newMemStoreClock(c.now)
	return &ProxyManager{run: casRunner{store: store, ttl: c.ttl}, now: c.now}
}
