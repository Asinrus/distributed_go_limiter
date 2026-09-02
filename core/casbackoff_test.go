package core

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// No AC id: retry pacing is an implementation property, not a behavioral
// criterion (ai-context/tasks/core-cas-retry-hardening.md). These are white-box
// so they can reach maxCASRetries/casBackoff/casRunner directly and assert
// pacing through the injected core.WithCASBackoff hook instead of sleeping.

func backoffTestConfig() Config {
	return NewConfig(NewBandwidth(10).RefillGreedy(1, time.Second))
}

// countingLoserStore always loses the compare-and-swap race and counts how many
// times it was asked for the current state, i.e. the number of attempts made.
type countingLoserStore struct {
	mu    sync.Mutex
	calls int
}

func (s *countingLoserStore) GetState(context.Context, string) ([]byte, bool, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return nil, false, nil
}

func (s *countingLoserStore) CompareAndSwap(context.Context, string, []byte, []byte, time.Duration) (bool, error) {
	return false, nil
}

func (s *countingLoserStore) Remove(context.Context, string) error { return nil }

func (s *countingLoserStore) attempts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// loseNTimesStore loses the first n compare-and-swap attempts, then wins.
type loseNTimesStore struct {
	n   int
	get int
}

func (s *loseNTimesStore) GetState(context.Context, string) ([]byte, bool, error) {
	s.get++
	return nil, false, nil
}

func (s *loseNTimesStore) CompareAndSwap(context.Context, string, []byte, []byte, time.Duration) (bool, error) {
	return s.get > s.n, nil
}

func (s *loseNTimesStore) Remove(context.Context, string) error { return nil }

// onceSwapStore always wins on the first try: the uncontended common case.
type onceSwapStore struct {
	getCalls, swapCalls int
}

func (s *onceSwapStore) GetState(context.Context, string) ([]byte, bool, error) {
	s.getCalls++
	return nil, false, nil
}

func (s *onceSwapStore) CompareAndSwap(context.Context, string, []byte, []byte, time.Duration) (bool, error) {
	s.swapCalls++
	return true, nil
}

func (s *onceSwapStore) Remove(context.Context, string) error { return nil }

// sleepSpy records every delay the retry loop asked to wait, so pacing is
// asserted without ever sleeping in wall-clock time.
type sleepSpy struct {
	mu     sync.Mutex
	delays []time.Duration
}

func (s *sleepSpy) sleep(_ context.Context, d time.Duration) error {
	s.mu.Lock()
	s.delays = append(s.delays, d)
	s.mu.Unlock()
	return nil
}

func TestCASBackoff_ExhaustionMakesExactlyMaxRetriesAttempts(t *testing.T) {
	store := &countingLoserStore{}
	pm := NewCASProxyManager(store, WithCASBackoff(func(context.Context, time.Duration) error { return nil }))
	b, err := pm.Bucket("k", backoffTestConfig())
	if err != nil {
		t.Fatal(err)
	}

	_, err = b.TryConsume(context.Background(), 1)
	if !errors.Is(err, ErrCASRetriesExhausted) {
		t.Fatalf("err = %v, want ErrCASRetriesExhausted", err)
	}
	if !errors.Is(err, ErrStore) {
		t.Fatalf("err = %v, want it to also wrap ErrStore", err)
	}
	if got := store.attempts(); got != maxCASRetries {
		t.Fatalf("attempts = %d, want exactly maxCASRetries=%d", got, maxCASRetries)
	}
}

func TestCASBackoff_UncontendedPathMakesOneAttemptNoDelay(t *testing.T) {
	store := &onceSwapStore{}
	spy := &sleepSpy{}
	pm := NewCASProxyManager(store, WithCASBackoff(spy.sleep))
	b, err := pm.Bucket("k", backoffTestConfig())
	if err != nil {
		t.Fatal(err)
	}

	ok, err := b.TryConsume(context.Background(), 1)
	if err != nil || !ok {
		t.Fatalf("TryConsume() = (%v, %v), want (true, nil)", ok, err)
	}
	if store.getCalls != 1 || store.swapCalls != 1 {
		t.Fatalf("get=%d swap=%d calls, want exactly 1 each", store.getCalls, store.swapCalls)
	}
	if len(spy.delays) != 0 {
		t.Fatalf("uncontended path slept %d times, want 0: %v", len(spy.delays), spy.delays)
	}
}

func TestCASBackoff_RetryDelaysGrowAndAreNotIdentical(t *testing.T) {
	store := &loseNTimesStore{n: 8}
	spy := &sleepSpy{}
	pm := NewCASProxyManager(store, WithCASBackoff(spy.sleep))
	b, err := pm.Bucket("k", backoffTestConfig())
	if err != nil {
		t.Fatal(err)
	}

	ok, err := b.TryConsume(context.Background(), 1)
	if err != nil || !ok {
		t.Fatalf("TryConsume() = (%v, %v), want (true, nil)", ok, err)
	}
	if len(spy.delays) != 8 {
		t.Fatalf("got %d backoff delays, want 8", len(spy.delays))
	}

	seen := map[time.Duration]bool{}
	for _, d := range spy.delays {
		seen[d] = true
	}
	if len(seen) < 2 {
		t.Fatalf("all backoff delays identical: %v (fixed backoff resynchronizes the herd instead of dispersing it)", spy.delays)
	}

	// Full jitter draws uniformly under a per-attempt ceiling that doubles up to
	// backoffCap, so the average of the later (larger-ceiling) delays should be
	// well above the average of the earliest ones. With ceilings 1,2,4ms vs
	// 16,32,32ms this holds under virtually any draw.
	var early, late time.Duration
	for _, d := range spy.delays[:3] {
		early += d
	}
	for _, d := range spy.delays[5:8] {
		late += d
	}
	if late <= early {
		t.Fatalf("backoff did not grow: early delays %v, late delays %v", spy.delays[:3], spy.delays[5:8])
	}
}

func TestCASBackoff_CancelledDuringDelayReturnsPromptly(t *testing.T) {
	store := &countingLoserStore{}
	ctx, cancel := context.WithCancel(context.Background())
	var sleepCalls int
	sleep := func(sctx context.Context, _ time.Duration) error {
		sleepCalls++
		cancel() // cancellation arrives mid-delay
		return sctx.Err()
	}
	pm := NewCASProxyManager(store, WithCASBackoff(sleep))
	b, err := pm.Bucket("k", backoffTestConfig())
	if err != nil {
		t.Fatal(err)
	}

	_, err = b.TryConsume(ctx, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if errors.Is(err, ErrCASRetriesExhausted) {
		t.Fatalf("must stop on cancellation, not run out the retry budget: %v", err)
	}
	if sleepCalls != 1 {
		t.Fatalf("sleep called %d times, want exactly 1 (stop at the first cancellation)", sleepCalls)
	}
	if got := store.attempts(); got != 1 {
		t.Fatalf("store contacted %d times, want exactly 1 (no attempt after the cancelled delay)", got)
	}
}

// AC-42 regression: backoff must change pacing only, never the outcome.
func TestCASBackoff_ConcurrentConsumptionStillGrantsExactlyCapacity(t *testing.T) {
	const capacity, workers, each = 500, 20, 50 // demand (1000) exceeds capacity (500)
	pm := NewInMemoryProxyManager()
	cfg := NewConfig(NewBandwidth(capacity).RefillGreedy(1, time.Hour))

	var success atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := pm.Bucket("k", cfg)
			if err != nil {
				t.Error(err)
				return
			}
			for i := 0; i < each; i++ {
				ok, err := b.TryConsume(context.Background(), 1)
				if err != nil {
					t.Error(err)
					return
				}
				if ok {
					success.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	if got := success.Load(); got != capacity {
		t.Fatalf("granted %d tokens, want exactly capacity=%d", got, capacity)
	}
}
