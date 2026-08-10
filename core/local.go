package core

import (
	"context"
	"sync"
)

// LocalBucket is an in-memory Bucket guarded by a mutex. It implements the same
// Bucket interface as the distributed backends, so code written against Bucket
// runs unchanged on either.
//
// ponytail: sync.Mutex, not an atomic.Pointer CAS loop — measured, and lock-free
// is 1.2–1.5x slower on accepted consumes (the hot path) because publishing a new
// state allocates. If this ever needs to be faster the clock is the target, not
// the lock: monotonicNanos is 29ns of a 54ns TryConsume, the mutex only ~15ns.
type LocalBucket struct {
	mu    sync.Mutex
	state *bucketState
	now   func() int64
}

// NewLocalBucket builds an in-memory bucket from cfg. It returns ErrInvalidConfig
// if cfg has no positive capacity or no refill rate — validating here means every
// *LocalBucket that exists is usable, so no operation has to re-check.
func NewLocalBucket(cfg Config) (*LocalBucket, error) {
	if !cfg.valid() {
		return nil, ErrInvalidConfig
	}
	return newLocalBucketClock(cfg, monotonicNanos), nil
}

// newLocalBucketClock is NewLocalBucket with an injectable clock (tests).
func newLocalBucketClock(cfg Config, now func() int64) *LocalBucket {
	return &LocalBucket{state: newState(cfg.bandwidth, now()), now: now}
}

var _ Bucket = (*LocalBucket)(nil)
var _ reserver = (*LocalBucket)(nil)

func (b *LocalBucket) TryConsume(_ context.Context, n int64) (bool, error) {
	if err := validateConsume(n); err != nil {
		return false, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	ok, _ := b.state.tryConsume(b.now(), n)
	return ok, nil
}

func (b *LocalBucket) TryConsumeAndReturnRemaining(_ context.Context, n int64) (ConsumptionProbe, error) {
	if err := validateConsume(n); err != nil {
		return ConsumptionProbe{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	probe, _ := b.state.tryConsumeAndReturn(b.now(), n)
	return probe, nil
}

func (b *LocalBucket) EstimateAbilityToConsume(_ context.Context, n int64) (EstimationProbe, error) {
	if err := validateConsume(n); err != nil {
		return EstimationProbe{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state.estimate(b.now(), n), nil
}

func (b *LocalBucket) TryConsumeAsMuchAsPossible(_ context.Context, limit int64) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	consumed, _ := b.state.consumeAsMuch(b.now(), limit)
	return consumed, nil
}

func (b *LocalBucket) AvailableTokens(_ context.Context) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state.availableAt(b.now()), nil
}

func (b *LocalBucket) AsBlocking() *BlockingBucket {
	return &BlockingBucket{r: b}
}

func (b *LocalBucket) capacity() int64 { return b.state.bw.capacity }

func (b *LocalBucket) reserveTokens(_ context.Context, n, maxWaitNanos int64) (int64, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	wait, ok, _ := b.state.reserve(b.now(), n, maxWaitNanos)
	return wait, ok, nil
}
