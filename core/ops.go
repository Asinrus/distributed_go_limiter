package core

import "math"

// Operations on bucketState shared by LocalBucket (direct) and the distributed
// command executor. Each returns whether the state was modified so CAS backends
// can skip the write on read-only / rejected operations (rejections don't persist
// the refill — recomputing it from the older timestamp is equivalent).

func max0(n int64) int64 {
	if n < 0 {
		return 0
	}
	return n
}

// tryConsume refills to now and consumes n if available.
func (s *bucketState) tryConsume(now, n int64) (ok, modified bool) {
	s.refill(now)
	if n <= s.currentSize {
		s.currentSize -= n
		return true, true
	}
	return false, false
}

// tryConsumeAndReturn is tryConsume plus a full ConsumptionProbe.
func (s *bucketState) tryConsumeAndReturn(now, n int64) (ConsumptionProbe, bool) {
	s.refill(now)
	if n <= s.currentSize {
		s.currentSize -= n
		return ConsumptionProbe{
			Consumed:            true,
			RemainingTokens:     s.currentSize,
			NanosToWaitForReset: s.fullRefillNanos(now),
		}, true
	}
	return ConsumptionProbe{
		Consumed:             false,
		RemainingTokens:      max0(s.currentSize),
		NanosToWaitForRefill: s.delayNanosForConsume(n, now),
		NanosToWaitForReset:  s.fullRefillNanos(now),
	}, false
}

// estimate reports the ability to consume n without consuming. It refills (which
// is idempotent at a fixed now) but never takes tokens; modified is always false.
func (s *bucketState) estimate(now, n int64) EstimationProbe {
	s.refill(now)
	if n <= s.currentSize {
		return EstimationProbe{CanBeConsumed: true, RemainingTokens: s.currentSize}
	}
	return EstimationProbe{
		CanBeConsumed:        false,
		RemainingTokens:      max0(s.currentSize),
		NanosToWaitForRefill: s.delayNanosForConsume(n, now),
	}
}

// consumeAsMuch consumes up to limit tokens (limit <= 0 means all available).
func (s *bucketState) consumeAsMuch(now, limit int64) (consumed int64, modified bool) {
	s.refill(now)
	toConsume := s.currentSize
	if toConsume <= 0 {
		return 0, false
	}
	if limit > 0 && toConsume > limit {
		toConsume = limit
	}
	s.currentSize -= toConsume
	return toConsume, true
}

// availableAt refills to now and returns the token count.
func (s *bucketState) availableAt(now int64) int64 {
	s.refill(now)
	return s.currentSize
}

// reserve consumes n tokens (allowing the count to go negative — a reservation)
// iff they can be obtained within maxWaitNanos, and returns how long to wait for
// them. ok=false means n exceeds capacity or cannot be obtained in time; nothing
// is consumed in that case. Basis for BlockingBucket.Consume.
func (s *bucketState) reserve(now, n, maxWaitNanos int64) (waitNanos int64, ok, modified bool) {
	s.refill(now)
	delay := s.delayNanosForConsume(n, now)
	if delay == math.MaxInt64 || delay > maxWaitNanos {
		return 0, false, false
	}
	s.currentSize -= n // may go negative; future refills repay the reservation
	return delay, true, true
}
