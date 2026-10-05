package core_test

import (
	"context"
	"fmt"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
)

// Example shows the local, in-process path: no storage backend at all. A
// bucket of capacity 3 accepts exactly 3 immediate TryConsume(1) calls and
// rejects the next one.
func Example() {
	cfg := core.NewConfig(core.NewBandwidth(3).RefillGreedy(1, time.Second))
	bucket, err := core.NewLocalBucket(cfg)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	for i := 0; i < 4; i++ {
		ok, err := bucket.TryConsume(ctx, 1)
		if err != nil {
			panic(err)
		}
		fmt.Println(ok)
	}
	// Output:
	// true
	// true
	// true
	// false
}

// Example_providerSwap is the provider-swap claim made concrete: consume runs
// the exact same calling code — Bucket, TryConsume, nothing backend-specific —
// against a LocalBucket and against a ProxyManager-backed bucket. Swapping a
// ProxyManager to a real distributed backend (Redis, Postgres, MongoDB, etcd)
// only changes which constructor builds it; consume itself never changes.
func Example_providerSwap() {
	cfg := core.NewConfig(core.NewBandwidth(2).RefillGreedy(1, time.Second))
	ctx := context.Background()

	consume := func(b core.Bucket) {
		for i := 0; i < 3; i++ {
			ok, err := b.TryConsume(ctx, 1)
			if err != nil {
				panic(err)
			}
			fmt.Println(ok)
		}
	}

	local, err := core.NewLocalBucket(cfg)
	if err != nil {
		panic(err)
	}
	consume(local)

	pm := core.NewInMemoryProxyManager()
	distributed, err := pm.Bucket("example-key", cfg)
	if err != nil {
		panic(err)
	}
	consume(distributed)
	// Output:
	// true
	// true
	// false
	// true
	// true
	// false
}
