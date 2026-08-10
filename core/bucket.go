package core

import (
	"context"
	"errors"
	"time"
)

// Errors returned by bucket operations.
var (
	// ErrNonPositiveTokens is returned when a token count is < 1.
	ErrNonPositiveTokens = errors.New("core: tokens to consume must be positive")
	// ErrTokensExceedCapacity is returned by blocking Consume when the request can
	// never be satisfied because it exceeds the bucket capacity.
	ErrTokensExceedCapacity = errors.New("core: tokens to consume exceed bucket capacity")
	// ErrInvalidConfig is returned by NewLocalBucket / ProxyManager.Bucket when a
	// Config is not usable (non-positive capacity, or no refill rate set).
	ErrInvalidConfig = errors.New("core: invalid bucket configuration")
	// ErrStore reports that the backing store failed — I/O error, timeout,
	// cancellation, or unavailability. Every error crossing the store boundary
	// wraps it, and the backend's own error stays reachable via errors.Is/As.
	// core does not decide fail-open vs fail-closed; the caller checks
	// errors.Is(err, ErrStore). Distributed buckets only.
	ErrStore = errors.New("core: store failure")
)

// Bucket is the rate-limiter API, implemented identically by the local in-memory
// bucket and by every distributed backend — swapping the provider never changes
// calling code. Distributed implementations honour ctx and may return ErrStore;
// the local one ignores ctx and never does. Config is validated at construction,
// so the only argument error is ErrNonPositiveTokens.
type Bucket interface {
	// TryConsume takes n tokens if available, reporting whether it succeeded.
	TryConsume(ctx context.Context, n int64) (bool, error)
	// TryConsumeAndReturnRemaining is TryConsume plus remaining tokens and wait times.
	TryConsumeAndReturnRemaining(ctx context.Context, n int64) (ConsumptionProbe, error)
	// EstimateAbilityToConsume reports whether n tokens could be consumed, without
	// consuming them.
	EstimateAbilityToConsume(ctx context.Context, n int64) (EstimationProbe, error)
	// TryConsumeAsMuchAsPossible consumes up to limit tokens (limit <= 0 means all
	// currently available) and returns how many were consumed.
	TryConsumeAsMuchAsPossible(ctx context.Context, limit int64) (int64, error)
	// AvailableTokens returns the current token count (after refill).
	AvailableTokens(ctx context.Context) (int64, error)
	// AsBlocking returns a blocking view that can wait for tokens to accrue.
	AsBlocking() *BlockingBucket
}

// reserver is the capability BlockingBucket needs; implemented by both local and
// distributed buckets.
type reserver interface {
	reserveTokens(ctx context.Context, n, maxWaitNanos int64) (waitNanos int64, ok bool, err error)
	capacity() int64
}

// Local buckets use a monotonic, process-local clock; distributed buckets must
// use a wall-clock epoch so timestamps in shared state are comparable across nodes.

var processStart = time.Now()

func monotonicNanos() int64 { return int64(time.Since(processStart)) }

func wallNanos() int64 { return time.Now().UnixNano() }

func validateConsume(n int64) error {
	if n < 1 {
		return ErrNonPositiveTokens
	}
	return nil
}
