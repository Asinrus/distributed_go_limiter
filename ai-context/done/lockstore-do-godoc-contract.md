# Document LockStore.Do's error-propagation contract (Dev)

Ruling: `ai-context/tasks/q8-lockstore-do-error-contract.md`, ADR 0009.
Doc-only — no behaviour change, no new test, no TCK change.

## 1. `core/store.go`

Add to `LockStore.Do`'s godoc, after the existing description:

```go
	// If fn returns an error, Do must abandon the write and return that error
	// unmodified or wrapped with %w — never replaced, flattened with %v, or
	// swallowed. The executor uses errors.Is on Do's return value to tell a
	// callback failure (corrupt state) from a store outage; breaking the chain
	// misreports the former as ErrStore, and returning nil turns a failed
	// operation into a silent success.
```

## 2. `pgstore` — verify, don't change

`Store.Do` already wraps with `fmt.Errorf("pgstore: do %q: %w", key, err)` and
`doOnce` discards the rollback error so it cannot mask `fn`'s. Confirm this is
still true at the commit you edit; if it isn't, fix `pgstore` rather than
weakening the contract.

## Acceptance

- Godoc clause present on `core.LockStore.Do`.
- `cd core && go vet ./... && go test -race ./...` clean.
- `cd pgstore && go test ./...` clean (unchanged behaviour).
