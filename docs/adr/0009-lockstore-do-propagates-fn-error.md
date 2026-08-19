# `LockStore.Do` must propagate the callback's error; doc-only, not TCK-enforced

`lockRunner.execute` (`core/command.go`) hands `Do` a callback that can fail on
its own account — a decode failure on corrupt stored bytes is core's error, not
the store's — and it recovers that distinction with
`errors.Is(err, ownErr)` on `Do`'s return value. The interface never said `Do`
had to keep that chain intact, so the guard rested on an unstated assumption.
Ratified as a clause on `LockStore.Do`'s godoc:

> If `fn` returns an error, `Do` must abandon the write and return that error
> unmodified or wrapped with `%w` — never replaced, flattened with `%v`, or
> swallowed.

Both halves are load-bearing and fail differently. A store that flattens with
`%v` gets its callers' decode errors relabelled `core.ErrStore`, so a caller
degrading on store outages fails open on permanently corrupt state. A store
that returns `nil` after `fn` failed is worse: `errors.Is(nil, ownErr)` is
false, `storeErr(nil)` is nil, and a failed operation is reported as a
successful one. `pgstore` already complies — `Do` wraps with
`fmt.Errorf("pgstore: do %q: %w", key, err)` and discards the rollback error
rather than letting it mask `fn`'s.

**Relaxing core instead of constraining the store, rejected.** `ownErr` is
captured per call, so `if ownErr != nil { return ownErr }` would need no
contract at all. It is wrong for a store that retries internally: `fn` fails
once, the store calls it again successfully, `Do` returns nil, and core reports
a failure for an operation that succeeded. `errors.Is` is what keeps the `err
== nil` case honest, and it needs the chain.

**Promoting this to a TCK-enforced acceptance criterion, deferred.** `tck.Run`
takes a `func() *core.ProxyManager` and can only reach a backend through the
`Bucket` API — it cannot plant corrupt bytes, which is the only way to make a
real store's `fn` fail. Enforcing this would mean a second TCK entry point
taking the raw `LockStore`, for one implementation that already complies, while
the entire queued backend backlog (MongoDB, etcd, Hazelcast) implements
`CASStore` and is unaffected. Revisit when a second `LockStore` lands; the
in-package `stubLock` in `core/storeerr_test.go` covers the core-side guard
until then.
