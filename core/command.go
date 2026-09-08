package core

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"
)

// ErrCASRetriesExhausted is returned when compare-and-swap keeps losing the race.
// It wraps ErrStore: the store is healthy but the operation still did not complete
// through it, which is the same failure class for a caller deciding how to degrade.
var ErrCASRetriesExhausted = fmt.Errorf("%w: compare-and-swap retries exhausted", ErrStore)

// storeErr tags an error that crossed the store boundary, keeping the backend's own
// error unwrappable underneath — unless the context is done, in which case the
// context's own error is returned instead and the underlying one dropped. Both
// halves matter: a cancelled client is not a store outage, and this is the single
// place where that shape is decided, so a driver that notices the cancellation
// first cannot report it differently from the executor's own guards. The dropped
// error is almost always the driver's own context.Canceled
// (docs/adr/0015-cancellation-is-not-a-store-failure.md).
func storeErr(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return fmt.Errorf("%w: %w", ErrStore, err)
}

// defaultMaxCASRetries is the retry budget when the caller sets none. It bounds
// both the number of store round trips and, since losing attempts back off, the
// wall-clock cost of a call: the loop backs off before every retry but the first,
// so n attempts cost at most (n-1)*backoffCap of sleep — 63*32ms = 2016ms at this
// default — plus n GetState/CompareAndSwap round trips. WithMaxCASRetries trades
// the two off.
//
// Measured against core/tck's AC-42 (8 workers x 250 ops on one hot key) on
// every real CASStore backend, not just MemStore, where a lost race costs a
// network round trip rather than nanoseconds: 16 failed against mongostore
// with retries exhausted, and 32 flaked the same way on a repeat run. 64 held
// across 2 redis runs, 3 mongostore runs, and 3 etcd runs — still a bounded
// latency budget, unlike the prior unbounded-cost 100 immediate attempts.
const defaultMaxCASRetries = 64

const (
	backoffBase = time.Millisecond      // delay before the first retry
	backoffCap  = 32 * time.Millisecond // delay stops growing past this
)

// casBackoff picks the delay before the attempt-th retry (attempt >= 1) using full
// jitter: uniform over [0, min(cap, base*2^(attempt-1))). Full jitter, rather than a
// fixed or capped-exponential-only delay, is what actually disperses a losing herd —
// a fixed backoff has every loser retry in lockstep and just resynchronizes them one
// step later.
func casBackoff(attempt int) time.Duration {
	d := backoffBase << uint(attempt-1)
	if d <= 0 || d > backoffCap { // <=0 catches shift overflow for large attempt
		d = backoffCap
	}
	return time.Duration(rand.Int63n(int64(d)))
}

// sleepCtx blocks for d or until ctx is done, whichever comes first, so a backoff
// delay never outlives a cancelled caller. The default casRunner.sleep.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// bucketOp mutates a bucketState and reports whether it changed, so read-only and
// rejected ops skip the write. The same op body runs on every provider.
type bucketOp func(s *bucketState) (modified bool)

// runner executes a bucketOp against a provider's state for a key: casRunner
// (optimistic retry) or lockRunner (exclusive lock).
type runner interface {
	execute(ctx context.Context, key string, cfg Config, now int64, op bucketOp) error
	remove(ctx context.Context, key string) error
}

// --- compare-and-swap (Redis, in-memory) ------------------------------------

type casRunner struct {
	store      CASStore
	ttl        time.Duration
	maxRetries int
	// sleep waits out a backoff delay, cancellable via ctx. Defaults to sleepCtx;
	// overridden in tests so backoff pacing is asserted without sleeping in
	// wall-clock time.
	sleep func(ctx context.Context, d time.Duration) error
}

func (r casRunner) execute(ctx context.Context, key string, cfg Config, now int64, op bucketOp) error {
	for i := 0; i < r.maxRetries; i++ {
		// Checked here rather than left to the store: uniform across every backend
		// instead of dependent on whether that driver's client honours ctx, and it
		// catches cancellation arriving between a successful read and the retry too.
		if err := ctx.Err(); err != nil {
			return err
		}
		if i > 0 {
			// sleep only fails when ctx ends mid-delay, so the bare context error is
			// the whole error here — an interrupted backoff is a departed caller, not
			// a store failure (docs/adr/0015-cancellation-is-not-a-store-failure.md).
			if err := r.sleep(ctx, casBackoff(i)); err != nil {
				return err
			}
		}
		old, found, err := r.store.GetState(ctx, key)
		if err != nil {
			return storeErr(ctx, err)
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
			return storeErr(ctx, err)
		}
		if swapped {
			return nil
		}
		// lost the race — reload and retry
	}
	return ErrCASRetriesExhausted
}

func (r casRunner) remove(ctx context.Context, key string) error {
	return storeErr(ctx, r.store.Remove(ctx, key))
}

// --- exclusive lock (Postgres) ----------------------------------------------

type lockRunner struct {
	store LockStore
	ttl   time.Duration
}

func (r lockRunner) execute(ctx context.Context, key string, cfg Config, now int64, op bucketOp) error {
	// Same guard as casRunner: reject work before it starts rather than trust every
	// LockStore.Do implementation to notice cancellation on its own.
	if err := ctx.Err(); err != nil {
		return err
	}
	// Our own callback failure (a decode error) comes back out through Do's return
	// value; keep hold of it so it is not mistaken for a store failure.
	var ownErr error
	err := r.store.Do(ctx, key, r.ttl, func(old []byte, found bool) ([]byte, bool, error) {
		s, err := loadState(old, found, cfg, now)
		if err != nil {
			ownErr = err
			return nil, false, err
		}
		if !op(s) {
			return nil, false, nil
		}
		return encodeState(s), true, nil
	})
	if ownErr != nil && errors.Is(err, ownErr) {
		return ownErr
	}
	return storeErr(ctx, err)
}

func (r lockRunner) remove(ctx context.Context, key string) error {
	return storeErr(ctx, r.store.Remove(ctx, key))
}

// loadState decodes stored bytes, or builds a fresh full bucket when absent
// (the "reconstruct" recovery: a missing bucket is a new one).
func loadState(data []byte, found bool, cfg Config, now int64) (*bucketState, error) {
	if found {
		return decodeState(data, cfg.bandwidth)
	}
	return newState(cfg.bandwidth, now), nil
}
