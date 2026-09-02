package core

import (
	"context"
	"time"
)

// ProxyManager builds distributed Buckets backed by a storage provider. Every
// backend uses this same concrete type — only the Store differs — so swapping
// providers changes one constructor line and nothing else.
type ProxyManager struct {
	run runner
	now func() int64
}

type proxyConfig struct {
	now   func() int64
	ttl   time.Duration
	sleep func(ctx context.Context, d time.Duration) error
}

// ProxyOption configures a ProxyManager.
type ProxyOption func(*proxyConfig)

// WithClock overrides the time source (default: wall-clock epoch nanos, which all
// nodes share). Mainly for tests.
func WithClock(now func() int64) ProxyOption {
	return func(c *proxyConfig) { c.now = now }
}

// WithExpireAfterWrite sets a TTL applied on every write (0 = no expiry).
func WithExpireAfterWrite(ttl time.Duration) ProxyOption {
	return func(c *proxyConfig) { c.ttl = ttl }
}

// WithCASBackoff overrides how a CAS-backed ProxyManager waits between losing
// compare-and-swap attempts (default: a real, cancellable sleep). Only meaningful
// with NewCASProxyManager; a LockProxyManager never retries. Mainly for tests, so
// backoff pacing can be asserted without sleeping in wall-clock time.
func WithCASBackoff(sleep func(ctx context.Context, d time.Duration) error) ProxyOption {
	return func(c *proxyConfig) { c.sleep = sleep }
}

func newProxyConfig(opts ...ProxyOption) proxyConfig {
	c := proxyConfig{now: wallNanos, sleep: sleepCtx}
	for _, o := range opts {
		o(&c)
	}
	return c
}

// NewCASProxyManager builds a ProxyManager over a compare-and-swap store (Redis,
// in-memory). Backends call this from their own constructor.
func NewCASProxyManager(store CASStore, opts ...ProxyOption) *ProxyManager {
	c := newProxyConfig(opts...)
	return &ProxyManager{run: casRunner{store: store, ttl: c.ttl, sleep: c.sleep}, now: c.now}
}

// NewLockProxyManager builds a ProxyManager over an exclusive-lock store (Postgres).
func NewLockProxyManager(store LockStore, opts ...ProxyOption) *ProxyManager {
	c := newProxyConfig(opts...)
	return &ProxyManager{run: lockRunner{store: store, ttl: c.ttl}, now: c.now}
}

// Bucket returns a Bucket for key with the given configuration. Nodes sharing a
// key must use matching configs (config is not stored in v1). It returns
// ErrInvalidConfig if cfg has no positive capacity or no refill rate — validating
// here means every Bucket that exists is usable, so no operation has to re-check.
func (pm *ProxyManager) Bucket(key string, cfg Config) (Bucket, error) {
	if !cfg.valid() {
		return nil, ErrInvalidConfig
	}
	return &proxyBucket{pm: pm, key: key, cfg: cfg}, nil
}

// Remove deletes a bucket's stored state. Removing a missing key is a no-op.
func (pm *ProxyManager) Remove(ctx context.Context, key string) error {
	return pm.run.remove(ctx, key)
}

// proxyBucket is a Bucket whose operations run through the provider.
type proxyBucket struct {
	pm  *ProxyManager
	key string
	cfg Config
}

var _ Bucket = (*proxyBucket)(nil)
var _ reserver = (*proxyBucket)(nil)

// do runs op against the provider using one consistent timestamp for the operation.
func (pb *proxyBucket) do(ctx context.Context, op func(s *bucketState, now int64) bool) error {
	now := pb.pm.now()
	return pb.pm.run.execute(ctx, pb.key, pb.cfg, now, func(s *bucketState) bool {
		return op(s, now)
	})
}

func (pb *proxyBucket) TryConsume(ctx context.Context, n int64) (bool, error) {
	if err := validateConsume(n); err != nil {
		return false, err
	}
	var result bool
	err := pb.do(ctx, func(s *bucketState, now int64) bool {
		ok, modified := s.tryConsume(now, n)
		result = ok
		return modified
	})
	return result, err
}

func (pb *proxyBucket) TryConsumeAndReturnRemaining(ctx context.Context, n int64) (ConsumptionProbe, error) {
	if err := validateConsume(n); err != nil {
		return ConsumptionProbe{}, err
	}
	var result ConsumptionProbe
	err := pb.do(ctx, func(s *bucketState, now int64) bool {
		probe, modified := s.tryConsumeAndReturn(now, n)
		result = probe
		return modified
	})
	return result, err
}

func (pb *proxyBucket) EstimateAbilityToConsume(ctx context.Context, n int64) (EstimationProbe, error) {
	if err := validateConsume(n); err != nil {
		return EstimationProbe{}, err
	}
	var result EstimationProbe
	err := pb.do(ctx, func(s *bucketState, now int64) bool {
		result = s.estimate(now, n)
		return false // never modifies
	})
	return result, err
}

func (pb *proxyBucket) TryConsumeAsMuchAsPossible(ctx context.Context, limit int64) (int64, error) {
	var result int64
	err := pb.do(ctx, func(s *bucketState, now int64) bool {
		consumed, modified := s.consumeAsMuch(now, limit)
		result = consumed
		return modified
	})
	return result, err
}

func (pb *proxyBucket) AvailableTokens(ctx context.Context) (int64, error) {
	var result int64
	err := pb.do(ctx, func(s *bucketState, now int64) bool {
		result = s.availableAt(now)
		return false
	})
	return result, err
}

func (pb *proxyBucket) AsBlocking() *BlockingBucket { return &BlockingBucket{r: pb} }

func (pb *proxyBucket) capacity() int64 { return pb.cfg.bandwidth.capacity }

func (pb *proxyBucket) reserveTokens(ctx context.Context, n, maxWaitNanos int64) (int64, bool, error) {
	var wait int64
	var ok bool
	err := pb.do(ctx, func(s *bucketState, now int64) bool {
		w, o, modified := s.reserve(now, n, maxWaitNanos)
		wait, ok = w, o
		return modified
	})
	return wait, ok, err
}
