// Package storetest is the shared conformance suite for distributed backends.
// Every provider (in-memory, Redis, Postgres, MongoDB, etcd) runs the SAME
// suite against a *core.ProxyManager — this is how the provider-swap goal
// (AC-46) is verified: one test body, only the constructor differs.
//
// It is a normal (non-_test.go) package, not a _test.go file, because Go
// cannot share test code across module boundaries and each backend lives in
// its own module. It uses only core's exported API, since what it proves is
// a claim about the public surface. Uses the stdlib testing package only, so
// importing it adds no external dependency to core.
package storetest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
)

var keyCounter atomic.Int64

func newKey() string {
	return fmt.Sprintf("storetest-%d-%d", time.Now().UnixNano(), keyCounter.Add(1))
}

// slowConfig has a capacity but a negligible refill rate, so token accounting is
// exact over a test's short wall-clock duration.
func slowConfig(capacity int64) core.Config {
	return core.NewConfig(core.NewBandwidth(capacity).RefillGreedy(1, time.Hour))
}

// fakeClock is a controllable clock, so cases that need refill to actually
// happen can advance time by calling advance instead of sleeping — driving
// the algorithm's own math rather than waiting on the wall clock.
type fakeClock struct{ ns atomic.Int64 }

func newFakeClock() *fakeClock {
	c := &fakeClock{}
	c.ns.Store(time.Now().UnixNano())
	return c
}

func (c *fakeClock) now() int64              { return c.ns.Load() }
func (c *fakeClock) advance(d time.Duration) { c.ns.Add(int64(d)) }

// Run executes the full conformance suite against the ProxyManager built by
// factory, which must forward now to the underlying store via
// core.WithProxyOption(core.WithClock(now)) (every backend's WithProxyOption
// does this identically — see redisstore.WithProxyOption and siblings). Call
// it from each backend's test with a container-backed manager.
func Run(t *testing.T, factory func(now func() int64) *core.ProxyManager) {
	t.Helper()
	t.Run("AC41_KeyIsolation", func(t *testing.T) { testKeyIsolation(t, factory(newFakeClock().now)) })
	t.Run("AC42_ConsistencyUnderContention", func(t *testing.T) { testConsistency(t, factory(newFakeClock().now)) })
	t.Run("AC43_AtomicInit", func(t *testing.T) { testAtomicInit(t, factory(newFakeClock().now)) })
	t.Run("AC44_Removal", func(t *testing.T) { testRemoval(t, factory(newFakeClock().now)) })
	t.Run("AC56_ProbesAndEstimate", func(t *testing.T) { testProbes(t, factory) })
	t.Run("AC57_ConsumeAsMuchAsPossible", func(t *testing.T) { testConsumeAsMuchAsPossible(t, factory) })
	t.Run("AC58_BlockingConsume", func(t *testing.T) { testBlockingConsume(t, factory) })
	t.Run("AC59_ClampAndRefundAcrossBackends", func(t *testing.T) { testClampAndRefund(t, factory) })
}

// AC-41: different keys are independent buckets.
func testKeyIsolation(t *testing.T, pm *core.ProxyManager) {
	ctx := context.Background()
	cfg := slowConfig(10)
	b1 := mustBucket(t, pm, newKey(), cfg)
	b2 := mustBucket(t, pm, newKey(), cfg)

	for i := 0; i < 10; i++ {
		ok, err := b1.TryConsume(ctx, 1)
		mustNoErr(t, err)
		if !ok {
			t.Fatalf("b1 consume %d unexpectedly failed", i)
		}
	}
	ok, err := b1.TryConsume(ctx, 1)
	mustNoErr(t, err)
	if ok {
		t.Fatal("b1 should be empty")
	}
	av2, err := b2.AvailableTokens(ctx)
	mustNoErr(t, err)
	if av2 != 10 {
		t.Fatalf("b2 must be untouched, available=%d want 10", av2)
	}
}

// AC-42: N concurrent proxies over the SAME key never lose or double updates —
// available == capacity - successful consumes exactly.
func testConsistency(t *testing.T, pm *core.ProxyManager) {
	ctx := context.Background()
	const capacity, workers, each = 5000, 8, 250
	key := newKey()
	cfg := slowConfig(capacity)

	var success atomic.Int64
	var errOnce sync.Once
	var firstErr error
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := pm.Bucket(key, cfg) // each "node" builds its own proxy
			if err != nil {
				errOnce.Do(func() { firstErr = err })
				return
			}
			for i := 0; i < each; i++ {
				ok, err := b.TryConsume(ctx, 1)
				if err != nil {
					errOnce.Do(func() { firstErr = err })
					return
				}
				if ok {
					success.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		t.Fatalf("unexpected error under contention: %v", firstErr)
	}
	if got := success.Load(); got != workers*each {
		t.Fatalf("expected all %d consumes to succeed (< capacity), got %d", workers*each, got)
	}
	av, err := mustBucket(t, pm, key, cfg).AvailableTokens(ctx)
	mustNoErr(t, err)
	if want := int64(capacity - workers*each); av != want {
		t.Fatalf("available=%d want %d (no lost/double updates)", av, want)
	}
}

// AC-43: racing first-use on a fresh key initializes exactly once.
func testAtomicInit(t *testing.T, pm *core.ProxyManager) {
	ctx := context.Background()
	const capacity, parallel = 100, 50
	key := newKey()
	cfg := slowConfig(capacity)

	var success atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			b, err := pm.Bucket(key, cfg)
			if err != nil {
				return // counted as a failure by the success total below
			}
			if ok, err := b.TryConsume(ctx, 1); err == nil && ok {
				success.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if success.Load() != parallel {
		t.Fatalf("expected %d successful consumes, got %d", parallel, success.Load())
	}
	av, err := mustBucket(t, pm, key, cfg).AvailableTokens(ctx)
	mustNoErr(t, err)
	if want := int64(capacity - parallel); av != want {
		t.Fatalf("available=%d want %d after atomic init", av, want)
	}
}

// AC-44: Remove clears state; removing a missing key is a no-op.
func testRemoval(t *testing.T, pm *core.ProxyManager) {
	ctx := context.Background()
	cfg := slowConfig(10)
	key := newKey()
	b := mustBucket(t, pm, key, cfg)

	for i := 0; i < 5; i++ {
		ok, err := b.TryConsume(ctx, 1)
		mustNoErr(t, err)
		if !ok {
			t.Fatalf("consume %d failed", i)
		}
	}
	if err := pm.Remove(ctx, key); err != nil {
		t.Fatalf("remove: %v", err)
	}
	av, err := mustBucket(t, pm, key, cfg).AvailableTokens(ctx)
	mustNoErr(t, err)
	if av != 10 {
		t.Fatalf("after remove, available=%d want fresh 10", av)
	}
	if err := pm.Remove(ctx, newKey()); err != nil {
		t.Fatalf("removing a missing key must be a no-op, got %v", err)
	}
}

// AC-56: EstimateAbilityToConsume and TryConsumeAndReturnRemaining, including
// a rejection's wait estimate and a refill observed through the injected
// clock — no wall-clock sleep.
func testProbes(t *testing.T, factory func(now func() int64) *core.ProxyManager) {
	ctx := context.Background()
	clk := newFakeClock()
	pm := factory(clk.now)
	const capacity = 10
	cfg := core.NewConfig(core.NewBandwidth(capacity).RefillGreedy(capacity, time.Hour))
	b := mustBucket(t, pm, newKey(), cfg)

	est, err := b.EstimateAbilityToConsume(ctx, capacity)
	mustNoErr(t, err)
	if !est.CanBeConsumed || est.RemainingTokens != capacity {
		t.Fatalf("estimate at full capacity = %+v", est)
	}

	probe, err := b.TryConsumeAndReturnRemaining(ctx, 4)
	mustNoErr(t, err)
	if !probe.Consumed || probe.RemainingTokens != capacity-4 {
		t.Fatalf("consume-and-return = %+v", probe)
	}

	// Asking for more than remains fails without consuming and reports a wait.
	est, err = b.EstimateAbilityToConsume(ctx, capacity)
	mustNoErr(t, err)
	if est.CanBeConsumed || est.RemainingTokens != capacity-4 || est.NanosToWaitForRefill <= 0 {
		t.Fatalf("estimate over remaining = %+v", est)
	}
	probe, err = b.TryConsumeAndReturnRemaining(ctx, capacity)
	mustNoErr(t, err)
	if probe.Consumed || probe.RemainingTokens != capacity-4 || probe.NanosToWaitForRefill <= 0 {
		t.Fatalf("rejected consume-and-return = %+v", probe)
	}

	// The two rejections above must not have mutated the bucket: the 4 already
	// spent are still the only ones gone.
	av, err := b.AvailableTokens(ctx)
	mustNoErr(t, err)
	if av != capacity-4 {
		t.Fatalf("available=%d want %d, a probe mutated state", av, capacity-4)
	}

	// Advance the injected clock past a full refill window — no wall-clock
	// sleep — and confirm the probes observe it.
	clk.advance(time.Hour)
	est, err = b.EstimateAbilityToConsume(ctx, capacity)
	mustNoErr(t, err)
	if !est.CanBeConsumed || est.RemainingTokens != capacity {
		t.Fatalf("estimate after refill = %+v", est)
	}
}

// AC-57: TryConsumeAsMuchAsPossible takes up to limit (or everything, limit
// <= 0), and refill (driven by the injected clock) replenishes what it can
// consume next.
func testConsumeAsMuchAsPossible(t *testing.T, factory func(now func() int64) *core.ProxyManager) {
	ctx := context.Background()
	clk := newFakeClock()
	pm := factory(clk.now)
	const capacity = 10
	cfg := core.NewConfig(core.NewBandwidth(capacity).RefillGreedy(capacity, time.Hour))
	b := mustBucket(t, pm, newKey(), cfg)

	got, err := b.TryConsumeAsMuchAsPossible(ctx, 3)
	mustNoErr(t, err)
	if got != 3 {
		t.Fatalf("limited consume got %d want 3", got)
	}

	got, err = b.TryConsumeAsMuchAsPossible(ctx, 0) // <= 0 means "all available"
	mustNoErr(t, err)
	if got != capacity-3 {
		t.Fatalf("unlimited consume got %d want %d", got, capacity-3)
	}

	got, err = b.TryConsumeAsMuchAsPossible(ctx, 5)
	mustNoErr(t, err)
	if got != 0 {
		t.Fatalf("consume on an empty bucket got %d want 0", got)
	}

	clk.advance(time.Hour)
	got, err = b.TryConsumeAsMuchAsPossible(ctx, -1) // any non-positive means "all"
	mustNoErr(t, err)
	if got != capacity {
		t.Fatalf("consume after refill got %d want %d", got, capacity)
	}
}

// AC-58: blocking Consume through a real backend — an immediate grant, a real
// wait for a reservation computed against the injected clock, and a
// cancellation mid-wait that returns context.Canceled without consuming.
func testBlockingConsume(t *testing.T, factory func(now func() int64) *core.ProxyManager) {
	ctx := context.Background()
	clk := newFakeClock()
	pm := factory(clk.now)
	// 10 tokens/s: enough tokens are available for immediate grants, and a
	// 5-token wait from empty is 500ms — short and reliable against a real
	// backend's own round-trip latency.
	cfg := core.NewConfig(core.NewBandwidth(10).RefillGreedy(10, time.Second))

	t.Run("immediate", func(t *testing.T) {
		b := mustBucket(t, pm, newKey(), cfg)
		if err := b.AsBlocking().Consume(ctx, 5); err != nil {
			t.Fatalf("immediate consume: %v", err)
		}
	})

	t.Run("waits then succeeds", func(t *testing.T) {
		b := mustBucket(t, pm, newKey(), cfg)
		if _, err := b.TryConsumeAsMuchAsPossible(ctx, 0); err != nil {
			t.Fatalf("drain: %v", err)
		}
		start := time.Now()
		err := b.AsBlocking().Consume(ctx, 5)
		if err != nil {
			t.Fatalf("blocking consume: %v", err)
		}
		if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
			t.Fatalf("consume returned after %v, expected a real wait around 500ms", elapsed)
		}
	})

	t.Run("cancelled mid-wait refunds", func(t *testing.T) {
		b := mustBucket(t, pm, newKey(), cfg)
		if _, err := b.TryConsumeAsMuchAsPossible(ctx, 0); err != nil {
			t.Fatalf("drain: %v", err)
		}
		before, err := b.AvailableTokens(ctx)
		mustNoErr(t, err)

		cctx, cancel := context.WithCancel(ctx)
		go func() {
			time.Sleep(50 * time.Millisecond) // reservation made, wait underway
			cancel()
		}()
		err = b.AsBlocking().Consume(cctx, 5)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled consume err = %v, want context.Canceled", err)
		}

		after, err := b.AvailableTokens(ctx)
		mustNoErr(t, err)
		if after != before {
			t.Fatalf("AC-54: available=%d want %d (reservation refunded)", after, before)
		}
	})
}

// AC-59: AvailableTokens never reports a negative count while a blocking
// reservation is outstanding (AC-53), across a real backend's own encode/
// decode round trip.
func testClampAndRefund(t *testing.T, factory func(now func() int64) *core.ProxyManager) {
	ctx := context.Background()
	clk := newFakeClock()
	pm := factory(clk.now)
	cfg := core.NewConfig(core.NewBandwidth(10).RefillGreedy(10, time.Second))
	b := mustBucket(t, pm, newKey(), cfg)
	if _, err := b.TryConsumeAsMuchAsPossible(ctx, 0); err != nil {
		t.Fatalf("drain: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = b.AsBlocking().Consume(context.Background(), 5) // ~500ms wait
	}()

	deadline := time.Now().Add(700 * time.Millisecond)
	for time.Now().Before(deadline) {
		avail, err := b.AvailableTokens(context.Background())
		mustNoErr(t, err)
		if avail != 0 {
			t.Fatalf("must never observe a negative available count, got %d", avail)
		}
		select {
		case <-done:
			return
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("Consume did not finish in time")
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// mustBucket builds a bucket or fails the test. Test-goroutine only — t.Fatalf
// from a spawned goroutine is illegal, so the concurrent cases build inline.
func mustBucket(t *testing.T, pm *core.ProxyManager, key string, cfg core.Config) core.Bucket {
	t.Helper()
	b, err := pm.Bucket(key, cfg)
	if err != nil {
		t.Fatalf("bucket %q: %v", key, err)
	}
	return b
}
