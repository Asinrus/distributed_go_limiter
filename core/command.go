package core

import (
	"context"
	"errors"
	"time"
)

// ErrCASRetriesExhausted is returned when compare-and-swap keeps losing the race.
var ErrCASRetriesExhausted = errors.New("core: compare-and-swap retries exhausted")

const maxCASRetries = 100

// bucketOp mutates a bucketState and reports whether it changed (so read-only /
// rejected ops skip the write). The same op body runs on every provider — this is
// the analog of bucket4j's RemoteCommand.
type bucketOp func(s *bucketState) (modified bool)

// runner executes a bucketOp against a provider's state for a key. Two impls:
// casRunner (optimistic retry) and lockRunner (exclusive lock).
type runner interface {
	execute(ctx context.Context, key string, cfg Config, now int64, op bucketOp) error
	remove(ctx context.Context, key string) error
}

// --- compare-and-swap (Redis, in-memory) ------------------------------------

type casRunner struct {
	store CASStore
	ttl   time.Duration
}

func (r casRunner) execute(ctx context.Context, key string, cfg Config, now int64, op bucketOp) error {
	for i := 0; i < maxCASRetries; i++ {
		old, found, err := r.store.GetState(ctx, key)
		if err != nil {
			return err
		}
		s, err := loadState(old, found, cfg, now)
		if err != nil {
			return err
		}
		if !op(s) {
			return nil // read-only / rejected: no write needed
		}
		var expected []byte
		if found {
			expected = old // nil expected => create-if-absent (SET NX)
		}
		swapped, err := r.store.CompareAndSwap(ctx, key, expected, encodeState(s), r.ttl)
		if err != nil {
			return err
		}
		if swapped {
			return nil
		}
		// lost the race — reload and retry
	}
	return ErrCASRetriesExhausted
}

func (r casRunner) remove(ctx context.Context, key string) error { return r.store.Remove(ctx, key) }

// --- exclusive lock (Postgres) ----------------------------------------------

type lockRunner struct {
	store LockStore
	ttl   time.Duration
}

func (r lockRunner) execute(ctx context.Context, key string, cfg Config, now int64, op bucketOp) error {
	return r.store.Do(ctx, key, r.ttl, func(old []byte, found bool) ([]byte, bool, error) {
		s, err := loadState(old, found, cfg, now)
		if err != nil {
			return nil, false, err
		}
		if !op(s) {
			return nil, false, nil
		}
		return encodeState(s), true, nil
	})
}

func (r lockRunner) remove(ctx context.Context, key string) error { return r.store.Remove(ctx, key) }

// loadState decodes stored bytes, or builds a fresh full bucket when absent
// (the "reconstruct" recovery: a missing bucket is a new one).
func loadState(data []byte, found bool, cfg Config, now int64) (*bucketState, error) {
	if found {
		return decodeState(data, cfg.bandwidth)
	}
	return newState(cfg.bandwidth, now), nil
}
