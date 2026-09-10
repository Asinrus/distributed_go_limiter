# Regression guard: every vendor backend honours a cancelled context

**Status: un-deferred, tier 2.** It was blocked on whether etcd stays a main
backend; that is answered — etcd stays, as a peer, and there are no backend
tiers at all (`ai-context/done/etcd-as-a-main-backend.md`). So this covers four
backends, and the count is settled rather than pending.

**The premise this task was first written on is false — corrected.** It was
scoped as a bug hunt, expecting at least one backend to drop the property. A
fact-check found all four already satisfy `errors.Is(err, context.Canceled)`,
etcd included. Verified in this repo's own pinned versions:

- **etcd** `go.etcd.io/etcd/client/v3@v3.5.16` — `ContextError`
  (`client.go:578-598`) substitutes the raw `ctx.Err()` sentinel for a
  `codes.Canceled` / `codes.DeadlineExceeded` gRPC status, and every KV path
  routes through it (`kv.go:115,120,125,131`, `txn.go:147`). This was the
  predicted failure; it is not one, and the earlier claim that gRPC status
  codes would leak through is **struck**.
- **redis** `go-redis/v9@v9.6.1` — `internal/pool/pool.go:302-304` returns a
  bare `ctx.Err()` before taking a pool turn.
- **pgx** `v5@v5.7.1` — `pgconn/errors.go:130-137` normalises a cancelled
  context to `context.Canceled` and a deadline to `&errTimeout{err: ctx.Err()}`,
  which has an `Unwrap` (`pgconn/errors.go:183`), so `errors.Is` holds on both.
- **mongo** `mongo-driver@v1.17.1` — `x/mongo/driver/operation.go:788-789`
  assigns `err = ctx.Err()` before attempting the round trip.

So this is a **regression guard, not a fix**: what it buys is that a driver
upgrade or a fifth backend cannot silently drop the property. That is worth
having — cheap pluggability is the product (roadmap owner ruling 3) — but it is
tier 2 work, not the tier 1 bug hunt it was filed as. Nothing in the repo
exercises a cancelled context against a real backend today (checked: no
`context.WithCancel`/`WithTimeout`/`WithDeadline` in any backend test or in
`core/storetest`), which is the actual gap.

**Why AC-50 does not already cover it.** AC-50's guard sits in the executor
(`core/command.go:44,84`), *above* the store. Drive a cancelled context through
`Bucket.TryConsume` on any backend and the guard answers first — every backend
passes, including one whose store ignores its context entirely. A test written
at the `Bucket` level proves the executor and says nothing about the vendor. The
assertion has to be made against the `CASStore` / `LockStore` value directly.

**What must hold.** Every store method takes a `ctx` and every one of them is
part of the contract at `core/store.go:15,20,22` (CAS) and `core/store.go:40,43`
(lock). With an already-cancelled or already-expired context, each must return a
non-nil error and must not perform its write.

**In scope**
- A store-level entry point in `core/storetest` beside the existing
  bucket-level one (`core/storetest/storetest.go`'s `Run`) —
  `storetest.RunCASStore` / `storetest.RunLockStore`, taking a
  factory for the store value. One package, no second harness, stdlib `testing`
  only, same as today.
- Each of `redisstore`, `pgstore`, `mongostore`, `etcdstore` calls it from the
  container test it already has (`redisstore/redisstore_test.go:40` and
  siblings), against the same container as `storetest.Run`.
- **A backend that fails is fixed in the backend, not by weakening the case.**
  The fix is translation: return the context's error rather than the transport's
  own. No current backend needs it — all four pass today per the audit above —
  so the first failure will be a driver upgrade or a new backend, which is
  precisely what the guard exists to catch.

**Reaching the store: use `export_test.go`, do not widen the public API.**
`pgstore/pgstore.go:247` and `mongostore/mongostore.go:132` export `NewStore`;
`redisstore/redisstore.go:37` and `etcdstore/etcdstore.go:16` keep `store`
unexported and offer only `NewProxyManager`
(`redisstore/redisstore.go:110`, `etcdstore/etcdstore.go:107`), and all four
test files are external (`package redisstore_test`). An internal
`export_test.go` in each of the two closed packages reaches the store without
adding public surface. Whether that asymmetry should be closed for *users* is a
real question — a caller building its own manager cannot reach the Redis store
today — but it is a public-API decision and not this task's to make as a
side-effect of a test. Raise it against roadmap row 8 if it matters.

**Error shape.** Assert `errors.Is(err, context.Canceled)` (and
`context.DeadlineExceeded`). `ErrStore` never appears at this boundary:
`storeErr` (`core/command.go:17`) is applied by the runner, above the store, so
a store method's error is the raw driver error. The one bucket-level case below
follows the ratified ruling — bare `ctx.Err()`, `errors.Is(err, ErrStore)`
false (`docs/adr/0015-cancellation-is-not-a-store-failure.md`).

**Relationship to roadmap row 7 (widen the suite): sibling, not absorbed.** Row 7
adds the missing *bucket-level* cases (probes, estimate,
`TryConsumeAsMuchAsPossible`, blocking `Consume`) to the existing
`storetest.Run(t, func() *core.ProxyManager)` signature. This adds a differently
shaped entry point on a different boundary, and needs `export_test.go` work in
two backends that row 7 does not. Either can land first; neither blocks the
other. Roadmap row 20, **tier 2** — a regression guard on a property that
currently holds is worth less than the tier 1 work it was ranked beside.

**Out of scope**
- Cancellation arriving *mid-flight*, after the request is on the wire. Not
  deterministically testable against a local container — the same race that
  kept AC-50 out of the k6 gate — and a flaky conformance case is worse than
  none. The case here is "already cancelled before the call", which is what
  proves the driver checks at all.
- Changing `CASStore` / `LockStore`. The contract already says `ctx`; this
  makes it enforced, not different.
- Any new backend. Four existing ones.

**AC id: AC-52**, kept even though the task is deferred — the id is already
cited from `ai-context/tasks/core-ctx-cancellation.md` and the roadmap, and
recycling it would be worse than parking it. AC-49 to AC-54 are claimed by
other in-flight work, some of it on unmerged branches; check `git ls-remote`
before assuming the next id is free.

**Done looks like:** each of the four backends runs the same store-level ctx
case against its own container, a backend that ignores its context fails it,
and the claim "every backend honours ctx" is a test result rather than a
sentence in a task file.

## Test scenarios

**Black-box (tester, Gate 2):** no new scenario, and no black-box surface —
the store boundary is below the HTTP handler and a correct backend is
indistinguishable from an incorrect one over the wire once the AC-50 executor
guard is in place. `e2e/k6/run-sandbox.sh` for each backend exits 0 with the
existing contract unchanged (5 accepted, 3 rejected; refill checks pass) as a
no-regression run only.

**Unit / integration (Dev):**
- AC-52, per backend, against the store value directly: `GetState`,
  `CompareAndSwap` and `Remove` (`Do` and `Remove` for `pgstore`) each return a
  non-nil error satisfying `errors.Is(err, context.Canceled)` when handed an
  already-cancelled context.
- AC-52: the same methods behave identically for an already-expired deadline,
  with `errors.Is(err, context.DeadlineExceeded)`.
- AC-52: `CompareAndSwap` on a cancelled context did **not** write — a later
  `GetState` on a live context shows the pre-call bytes. An error return that
  still mutated the store is the failure this case exists to catch, and it is
  not caught by asserting the error alone.
- AC-52: the same cases run green against `core.MemStore`, which the in-memory
  manager already exercises — the reference `CASStore` must pass the suite it
  defines.
- One bucket-level case per backend: a cancelled context through
  `Bucket.TryConsume` returns a bare `ctx.Err()` — `errors.Is(err,
  context.Canceled)` true, `errors.Is(err, core.ErrStore)` false, per
  `docs/adr/0015-cancellation-is-not-a-store-failure.md` — and leaves the token
  unspent.
