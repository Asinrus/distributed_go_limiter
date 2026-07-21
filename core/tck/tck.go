// Package tck is the shared conformance suite for distributed backends. Every
// provider (in-memory, Redis, Postgres) runs the SAME suite against a
// *core.ProxyManager — this is how the provider-swap goal (AC-46) is verified:
// one test body, only the constructor differs. Uses the stdlib testing package
// only, so importing it adds no external dependency to core.
package tck

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
)

var keyCounter atomic.Int64

func newKey() string {
	return fmt.Sprintf("tck-%d-%d", time.Now().UnixNano(), keyCounter.Add(1))
}

// slowConfig has a capacity but a negligible refill rate, so token accounting is
// exact over a test's short wall-clock duration.
func slowConfig(capacity int64) core.Config {
	return core.NewConfig(core.NewBandwidth(capacity).RefillGreedy(1, time.Hour))
}

// Run executes the full conformance suite against the ProxyManager built by
// factory. Call it from each backend's test with a container-backed manager.
func Run(t *testing.T, factory func() *core.ProxyManager) {
	t.Helper()
	t.Run("AC41_KeyIsolation", func(t *testing.T) { testKeyIsolation(t, factory()) })
	t.Run("AC42_ConsistencyUnderContention", func(t *testing.T) { testConsistency(t, factory()) })
	t.Run("AC43_AtomicInit", func(t *testing.T) { testAtomicInit(t, factory()) })
	t.Run("AC44_Removal", func(t *testing.T) { testRemoval(t, factory()) })
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
