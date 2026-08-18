# Q8 — ratify LockStore.Do's error-handling contract

**Origin:** `LockStore.Do` (`core/store.go:34-35`) doesn't specify what a
store must do with the callback (`fn`)'s error. `lockRunner.execute`
(`core/command.go:100`) uses `errors.Is` to distinguish a decode failure
(corrupt state) from a genuine store outage — that guard only works if `Do`
returns `fn`'s error unmodified or wrapped, never replaced. Proposed wording:

> `Do` must return `fn`'s error unmodified or wrapped, never replaced.

**Goal:** Architect ratifies the wording (or amends it) and decides whether
this is doc-only (godoc on `LockStore.Do`) or also an acceptance criterion
the shared TCK enforces against every `LockStore` implementation.

**Acceptance:** Wording finalized in `core/store.go`'s godoc. If promoted to
an AC, `core/tck` gets a test proving a store that replaces `fn`'s error
fails the TCK, and `pgstore` (the only current `LockStore`) still passes.
