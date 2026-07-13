// Package core defines the token-bucket API and algorithm for a distributed
// rate limiter. The token math lives here once; storage providers (local,
// Redis, Postgres) only supply an atomic Store. See docs/internal/project/ARCHITECTURE.md.
package core

import "time"

// Bandwidth is one immutable rate limit: a bucket of at most Capacity tokens,
// refilled RefillTokens every RefillPeriod. v1 supports a single Bandwidth per
// bucket (multi-bandwidth is deferred).
//
// Build with NewBandwidth(capacity) then RefillGreedy / RefillIntervally:
//
//	core.NewBandwidth(50).RefillGreedy(10, time.Second)
type Bandwidth struct {
	capacity          int64
	initialTokens     int64
	refillPeriodNanos int64
	refillTokens      int64
	refillIntervally  bool // false = greedy (continuous drip), true = whole chunks on boundaries
}

// NewBandwidth starts a bandwidth with the given capacity. Initial tokens default
// to capacity (a full bucket); override with WithInitialTokens. A refill mode
// (RefillGreedy or RefillIntervally) must be set before use.
func NewBandwidth(capacity int64) Bandwidth {
	return Bandwidth{capacity: capacity, initialTokens: capacity}
}

// RefillGreedy refills tokens continuously and proportionally to elapsed time:
// tokens tokens are added smoothly over each period.
func (b Bandwidth) RefillGreedy(tokens int64, period time.Duration) Bandwidth {
	b.refillTokens = tokens
	b.refillPeriodNanos = period.Nanoseconds()
	b.refillIntervally = false
	return b
}

// RefillIntervally refills in discrete chunks: the whole tokens amount is added
// once at the end of each period, nothing in between.
func (b Bandwidth) RefillIntervally(tokens int64, period time.Duration) Bandwidth {
	b.refillTokens = tokens
	b.refillPeriodNanos = period.Nanoseconds()
	b.refillIntervally = true
	return b
}

// WithInitialTokens overrides the starting token count (default = capacity).
func (b Bandwidth) WithInitialTokens(n int64) Bandwidth {
	b.initialTokens = n
	return b
}

// Capacity returns the maximum number of tokens.
func (b Bandwidth) Capacity() int64 { return b.capacity }

// valid reports whether the bandwidth is usable (positive capacity and a refill rate).
func (b Bandwidth) valid() bool {
	return b.capacity > 0 && b.refillTokens > 0 && b.refillPeriodNanos > 0
}
