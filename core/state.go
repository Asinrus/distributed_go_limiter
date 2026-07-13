package core

import "math"

// bucketState is the mutable per-bucket state. For a single bandwidth it is three
// int64s — a direct reduction of bucket4j's BucketState64BitsInteger:
//
//	lastRefillNanos : time of the last refill
//	currentSize     : available tokens
//	roundingError   : carried remainder that keeps integer greedy refill exact
//
// Only those three values are persisted (see codec.go); the Bandwidth is supplied
// by the client config at runtime.
type bucketState struct {
	lastRefillNanos int64
	currentSize     int64
	roundingError   int64
	bw              Bandwidth
}

// newState creates the initial state at nowNanos.
func newState(bw Bandwidth, nowNanos int64) *bucketState {
	return &bucketState{
		lastRefillNanos: nowNanos,
		currentSize:     bw.initialTokens,
		roundingError:   0,
		bw:              bw,
	}
}

// available returns the current token count (call refill first for a fresh value).
func (s *bucketState) available() int64 { return s.currentSize }

// consume subtracts n tokens (may drive the count negative for reservations).
func (s *bucketState) consume(n int64) { s.currentSize -= n }

// refill advances the bucket to nowNanos, adding accrued tokens. Ported verbatim
// (single bandwidth) from BucketState64BitsInteger.refill, including the greedy
// rounding-error carry and the intervally boundary truncation.
func (s *bucketState) refill(nowNanos int64) {
	prev := s.lastRefillNanos
	if nowNanos <= prev {
		return // clock went backward / no time passed
	}
	bw := s.bw

	if bw.refillIntervally {
		// Only refill on whole-period boundaries.
		correction := (nowNanos - prev) % bw.refillPeriodNanos
		nowNanos -= correction
	}
	if nowNanos <= prev {
		return
	}
	s.lastRefillNanos = nowNanos

	capacity := bw.capacity
	period := bw.refillPeriodNanos
	refillTokens := bw.refillTokens
	current := s.currentSize
	if current >= capacity {
		return // can happen after force-add overflow
	}

	duration := nowNanos - prev
	newSize := current

	if duration > period {
		elapsedPeriods := duration / period
		newSize += elapsedPeriods * refillTokens
		if newSize > capacity || newSize < current {
			// exceeded capacity or arithmetic overflow → clamp full
			s.resetFull()
			return
		}
		duration %= period
	}

	// Sub-period fractional refill with rounding-error carry.
	rounding := s.roundingError
	product := mulOrMax(refillTokens, duration)
	divided := product + rounding
	if divided < 0 || product == math.MaxInt64 {
		// overflow — drop to float arithmetic
		calc := int64(float64(duration) / float64(period) * float64(refillTokens))
		newSize += calc
		rounding = 0
	} else {
		calc := divided / period
		if calc == 0 {
			rounding = divided
		} else {
			newSize += calc
			rounding = divided % period
		}
	}

	if newSize >= capacity || newSize < current {
		s.resetFull()
		return
	}
	s.currentSize = newSize
	s.roundingError = rounding
}

func (s *bucketState) resetFull() {
	s.currentSize = s.bw.capacity
	s.roundingError = 0
}

// delayNanosForConsume returns nanoseconds until n tokens can be consumed given
// the current state at nowNanos. 0 if already possible; math.MaxInt64 if n exceeds
// what the bucket can ever hold or on arithmetic overflow. Call refill(now) first.
func (s *bucketState) delayNanosForConsume(n, nowNanos int64) int64 {
	if n > s.bw.capacity {
		return math.MaxInt64
	}
	if n <= s.currentSize {
		return 0
	}
	deficit := n - s.currentSize
	if deficit <= 0 {
		return math.MaxInt64 // overflow
	}
	bw := s.bw

	if bw.refillIntervally {
		next := s.lastRefillNanos + bw.refillPeriodNanos
		waitNext := next - nowNanos
		if deficit <= bw.refillTokens {
			return waitNext
		}
		deficit -= bw.refillTokens
		if deficit < bw.refillTokens {
			return waitNext + bw.refillPeriodNanos
		}
		periods := deficit / bw.refillTokens
		if deficit%bw.refillTokens != 0 {
			periods++
		}
		dn := mulOrMax(periods, bw.refillPeriodNanos)
		if dn == math.MaxInt64 {
			return math.MaxInt64
		}
		dn += waitNext
		if dn < 0 {
			return math.MaxInt64
		}
		return dn
	}

	// greedy
	divided := mulOrMax(bw.refillPeriodNanos, deficit)
	if divided == math.MaxInt64 {
		return int64(float64(deficit) / float64(bw.refillTokens) * float64(bw.refillPeriodNanos))
	}
	divided -= s.roundingError
	return divided / bw.refillTokens
}

// fullRefillNanos returns the time until the bucket is completely full.
func (s *bucketState) fullRefillNanos(nowNanos int64) int64 {
	if s.currentSize >= s.bw.capacity {
		return 0
	}
	return s.delayNanosForConsume(s.bw.capacity, nowNanos)
}

// mulOrMax returns x*y, or math.MaxInt64 on overflow — the Go analog of bucket4j's
// multiplyExactOrReturnMaxValue (assumes non-negative inputs, which holds here).
func mulOrMax(x, y int64) int64 {
	if x == 0 || y == 0 {
		return 0
	}
	r := x * y
	if r/y != x || r < 0 {
		return math.MaxInt64
	}
	return r
}
