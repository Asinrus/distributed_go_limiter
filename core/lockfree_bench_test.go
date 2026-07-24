package core

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Scratch benchmark: is a lock-free LocalBucket worth it?
//
// local.go carries a ponytail note deferring this until profiling shows
// contention. This file is that profiling. It is a benchmark only — nothing here
// is wired into the library.
//
// The design under test is the one the note names: atomic.Pointer[bucketState]
// plus a compare-and-swap retry loop. The token maths is NOT forked — it reuses
// the same ops.go methods as LocalBucket and the distributed path, which is the
// constraint that makes this worth considering at all.

type lockFreeBucket struct {
	state atomic.Pointer[bucketState]
	now   func() int64
}

func newLockFreeBucket(cfg Config, now func() int64) *lockFreeBucket {
	b := &lockFreeBucket{now: now}
	b.state.Store(newState(cfg.bandwidth, now()))
	return b
}

func (b *lockFreeBucket) TryConsume(_ context.Context, n int64) (bool, error) {
	if err := validateConsume(n); err != nil {
		return false, err
	}
	for {
		old := b.state.Load()

		// Trial copy on the stack: probe never has its address escape, so a
		// rejected consume costs one atomic load and no allocation at all.
		probe := *old
		ok, modified := probe.tryConsume(b.now(), n)
		if !modified {
			return ok, nil // rejection changes nothing — nothing to publish
		}

		// Only now pay for a heap cell, because only now is there a new version
		// to publish.
		fresh := new(bucketState)
		*fresh = probe
		if b.state.CompareAndSwap(old, fresh) {
			return ok, nil
		}
		// Lost the race: another goroutine published first. Retry with its state.
	}
}

// AvailableTokens is pure: refill the copy, report, publish nothing. The refill
// is idempotent from a given lastRefillNanos, so recomputing it next call is
// equivalent — the same trick ops.go already uses for the distributed estimate
// path (modified=false ⇒ no write).
func (b *lockFreeBucket) AvailableTokens(_ context.Context) (int64, error) {
	s := *b.state.Load()
	return s.availableAt(b.now()), nil
}

// --- benchmarks --------------------------------------------------------------

// acceptCfg: capacity so large every consume succeeds, so every operation
// mutates. This is the WORST case for CAS and the COMMON case in production —
// traffic under the limit is the normal state of a rate limiter.
func acceptCfg() Config {
	return NewConfig(NewBandwidth(1<<50).RefillGreedy(1<<20, time.Second))
}

// rejectCfg: empty bucket that refills too slowly to recover, so every consume
// is rejected and nothing is ever published.
func rejectCfg() Config {
	return NewConfig(NewBandwidth(1).RefillGreedy(1, time.Hour).WithInitialTokens(0))
}

func benchMutex(b *testing.B, cfg Config, parallel bool) {
	bk := NewLocalBucketMust(b, cfg)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	if parallel {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, _ = bk.TryConsume(ctx, 1)
			}
		})
		return
	}
	for i := 0; i < b.N; i++ {
		_, _ = bk.TryConsume(ctx, 1)
	}
}

func benchLockFree(b *testing.B, cfg Config, parallel bool) {
	bk := newLockFreeBucket(cfg, monotonicNanos)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	if parallel {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, _ = bk.TryConsume(ctx, 1)
			}
		})
		return
	}
	for i := 0; i < b.N; i++ {
		_, _ = bk.TryConsume(ctx, 1)
	}
}

func NewLocalBucketMust(tb testing.TB, cfg Config) *LocalBucket {
	tb.Helper()
	bk, err := NewLocalBucket(cfg)
	if err != nil {
		tb.Fatal(err)
	}
	return bk
}

func BenchmarkConsumeAcceptSerial(b *testing.B) {
	b.Run("mutex", func(b *testing.B) { benchMutex(b, acceptCfg(), false) })
	b.Run("lockfree", func(b *testing.B) { benchLockFree(b, acceptCfg(), false) })
}

func BenchmarkConsumeAcceptParallel(b *testing.B) {
	b.Run("mutex", func(b *testing.B) { benchMutex(b, acceptCfg(), true) })
	b.Run("lockfree", func(b *testing.B) { benchLockFree(b, acceptCfg(), true) })
}

func BenchmarkConsumeRejectParallel(b *testing.B) {
	b.Run("mutex", func(b *testing.B) { benchMutex(b, rejectCfg(), true) })
	b.Run("lockfree", func(b *testing.B) { benchLockFree(b, rejectCfg(), true) })
}

func BenchmarkAvailableTokensParallel(b *testing.B) {
	ctx := context.Background()
	b.Run("mutex", func(b *testing.B) {
		bk := NewLocalBucketMust(b, acceptCfg())
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, _ = bk.AvailableTokens(ctx)
			}
		})
	})
	b.Run("lockfree", func(b *testing.B) {
		bk := newLockFreeBucket(acceptCfg(), monotonicNanos)
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, _ = bk.AvailableTokens(ctx)
			}
		})
	})
}

// How much of the above is the clock rather than the lock?
func BenchmarkClockOnly(b *testing.B) {
	var sink int64
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sink = monotonicNanos()
	}
	_ = sink
}

// Pure token maths, no synchronisation and no clock: the floor either design
// is competing against.
func BenchmarkMathOnly(b *testing.B) {
	s := newState(acceptCfg().bandwidth, 0)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s.tryConsume(int64(i), 1)
	}
}

// AC-22 against the lock-free variant: under concurrency it must never
// over-grant. Frozen clock, so no refill can mask a lost update.
func TestLockFreeExactAccounting(t *testing.T) {
	clk := newFakeClock()
	const capacity, workers, each = 1000, 8, 1000
	b := newLockFreeBucket(NewConfig(NewBandwidth(capacity).RefillGreedy(1, time.Hour)), clk.now)
	ctx := context.Background()

	var success atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < each; i++ {
				if ok, _ := b.TryConsume(ctx, 1); ok {
					success.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	if got := success.Load(); got != capacity {
		t.Fatalf("over/under-granted: %d succeeded, want exactly %d", got, capacity)
	}
	avail, _ := b.AvailableTokens(ctx)
	if avail != 0 {
		t.Fatalf("available=%d want 0", avail)
	}
}
