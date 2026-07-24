package core

import (
	"context"
	"sync"
)

// LocalBucket is an in-memory Bucket guarded by a mutex — the hot path, no
// serialization. It implements the same Bucket interface as the distributed
// backends, so code written against Bucket runs unchanged on either.
//
// ponytail: a plain sync.Mutex around three int64s. The atomic.Pointer[bucketState]
// CAS loop this note used to recommend has since been built and measured — see
// lockfree_bench_test.go, which also proves it correct under -race. Verdict: keep
// the mutex.
//
// Lock-free is 1.2–1.5x SLOWER on accepted consumes, because publishing a new
// state costs a 64-byte allocation, and contention adds ~0.8 wasted retries per
// op on top. It wins only where nothing is published: rejections (5x) and
// AvailableTokens (7.5x at 8 cores, and it scales where the mutex degrades).
// Accepted consumes are the hot path — traffic under the limit is the normal
// state of a rate limiter — so the mutex wins the case that matters.
//
// If this ever does need to be faster, the clock is the target, not the lock:
// of a 54ns TryConsume, monotonicNanos is 29ns, the token maths 10ns, and the
// mutex only ~15ns. Perfect zero-cost locking would buy 28%; a coarser cached
// clock would buy more.
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
