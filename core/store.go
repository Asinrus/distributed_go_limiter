package core

import (
	"context"
	"time"
)

// A storage provider implements exactly one of CASStore or LockStore. The core
// command executor drives it; the token math is never in the store. See
// docs/internal/project/ARCHITECTURE.md.

// CASStore is the optimistic (compare-and-swap) provider contract — Redis and the
// in-memory store. State is an opaque byte blob keyed by string.
type CASStore interface {
	// GetState returns the stored bytes for key, or found=false if absent/expired.
	GetState(ctx context.Context, key string) (state []byte, found bool, err error)
	// CompareAndSwap writes newState iff the currently stored bytes equal old
	// (byte-for-byte). When old is nil it means "create only if absent". It returns
	// swapped=false (no error) when another writer won the race — the caller retries.
	// A positive ttl sets/refreshes an expire-after-write.
	CompareAndSwap(ctx context.Context, key string, old, newState []byte, ttl time.Duration) (swapped bool, err error)
	// Remove deletes the key (no error if absent).
	Remove(ctx context.Context, key string) error
}

// LockStore is the pessimistic (exclusive-lock) provider contract — Postgres
// (SELECT ... FOR UPDATE). The lock is held across the whole read-modify-write, so
// no retry loop is needed for correctness.
type LockStore interface {
	// Do runs fn while holding an exclusive lock on key. fn receives the current
	// state (found=false if absent) and returns the new bytes to persist with
	// write=true, or write=false to leave the state unchanged. A positive ttl sets
	// an expire-after-write on the persisted row.
	Do(ctx context.Context, key string, ttl time.Duration,
		fn func(old []byte, found bool) (newState []byte, write bool, err error)) error
	// Remove deletes the key (no error if absent).
	Remove(ctx context.Context, key string) error
}
