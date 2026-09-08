package core

import (
	"context"
	"errors"
	"testing"
	"time"
)

// No AC id: see the note in casbackoff_test.go. White-box so the configured
// budget can be compared against defaultMaxCASRetries, and reusing that file's
// countingLoserStore / sleepSpy fixtures.

// lockNoopStore is a LockStore that never runs its callback: enough to build a
// lock-backed ProxyManager and check option validation applies there too.
type lockNoopStore struct{}

func (lockNoopStore) Do(context.Context, string, time.Duration, func([]byte, bool) ([]byte, bool, error)) error {
	return nil
}

func (lockNoopStore) Remove(context.Context, string) error { return nil }

func TestCASRetryBudget_CustomBudgetReplacesTheDefault(t *testing.T) {
	const budget = 3
	store := &countingLoserStore{}
	spy := &sleepSpy{}
	pm := NewCASProxyManager(store, WithMaxCASRetries(budget), withSleep(spy.sleep))
	b, err := pm.Bucket("k", backoffTestConfig())
	if err != nil {
		t.Fatal(err)
	}

	_, err = b.TryConsume(context.Background(), 1)
	if !errors.Is(err, ErrCASRetriesExhausted) {
		t.Fatalf("err = %v, want ErrCASRetriesExhausted", err)
	}
	if got := store.attempts(); got != budget {
		t.Fatalf("attempts = %d, want exactly %d (the configured budget, not the default %d)",
			got, budget, defaultMaxCASRetries)
	}
	if len(spy.delays) != budget-1 {
		t.Fatalf("got %d backoff delays, want %d (one before every retry but the first)",
			len(spy.delays), budget-1)
	}
}

func TestCASRetryBudget_BudgetOfOneNeverBacksOff(t *testing.T) {
	store := &countingLoserStore{}
	spy := &sleepSpy{}
	pm := NewCASProxyManager(store, WithMaxCASRetries(1), withSleep(spy.sleep))
	b, err := pm.Bucket("k", backoffTestConfig())
	if err != nil {
		t.Fatal(err)
	}

	if _, err = b.TryConsume(context.Background(), 1); !errors.Is(err, ErrCASRetriesExhausted) {
		t.Fatalf("err = %v, want ErrCASRetriesExhausted", err)
	}
	if got := store.attempts(); got != 1 {
		t.Fatalf("attempts = %d, want exactly 1", got)
	}
	if len(spy.delays) != 0 {
		t.Fatalf("slept %d times with a budget of 1, want 0: %v", len(spy.delays), spy.delays)
	}
}

// A non-positive budget is caller misuse, rejected like any other invalid
// config and never reported as a store failure.
func TestCASRetryBudget_NonPositiveIsInvalidConfig(t *testing.T) {
	for _, n := range []int{0, -1} {
		for name, pm := range map[string]*ProxyManager{
			"cas":  NewCASProxyManager(&countingLoserStore{}, WithMaxCASRetries(n)),
			"lock": NewLockProxyManager(lockNoopStore{}, WithMaxCASRetries(n)),
		} {
			b, err := pm.Bucket("k", backoffTestConfig())
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("%s WithMaxCASRetries(%d): err = %v, want ErrInvalidConfig", name, n, err)
			}
			if errors.Is(err, ErrStore) {
				t.Fatalf("%s WithMaxCASRetries(%d): misuse must not be an ErrStore: %v", name, n, err)
			}
			if b != nil {
				t.Fatalf("%s WithMaxCASRetries(%d): got a usable Bucket back", name, n)
			}
		}
	}
}
