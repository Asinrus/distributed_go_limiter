# Does every vendor backend actually honour a cancelled context?

**The question, not a symptom.** `ai-context/tasks/core-ctx-cancellation.md`
(AC-50) makes `core` refuse to *start* work on a dead context, and says of the
backends: "the Redis, pgx, Mongo and etcd drivers all honour a cancelled
context and fail the round trip. But the honouring is then a property of each
driver rather than of this library." Nothing in this repo checks that claim.
Cheap pluggability is the product (roadmap owner ruling 3); a guarantee no test
holds a backend to is a guarantee the fifth backend quietly drops.

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
- A store-level entry point in `core/tck` beside the existing bucket-level one
  (`core/tck/tck.go:33`) — `tck.RunCASStore` / `tck.RunLockStore`, taking a
  factory for the store value. One package, no second harness, stdlib `testing`
  only, same as today.
- Each of `redisstore`, `pgstore`, `mongostore`, `etcdstore` calls it from the
  container test it already has (`redisstore/redisstore_test.go:40` and
  siblings), against the same container as `tck.Run`.
- **A backend that fails is fixed in the backend, not by weakening the case.**
  The fix is translation: return the context's error rather than the transport's
  own. `etcdstore` is the likeliest failure — gRPC surfaces cancellation as a
  `status.Error(codes.Canceled)`, which does not necessarily satisfy
  `errors.Is(err, context.Canceled)`. That is exactly the finding worth having.

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

**Error shape: store-level assertions are independent of the open dispute.**
Assert `errors.Is(err, context.Canceled)` (and `context.DeadlineExceeded`).
`ErrStore` never appears here: `storeErr` (`core/command.go:17`) is applied by
the runner, above the store, so a store method's error is the raw driver error
under either candidate ruling. This task is therefore **not blocked** by the
`ErrStore`-vs-bare disagreement between
`docs/adr/0015-cancellation-is-not-a-store-failure.md` and open PR #12. The one
bucket-level case below is the exception, and it must reference whichever shape
wins rather than hardcoding one.

**Relationship to roadmap row 7 (widen the TCK): sibling, not absorbed.** Row 7
adds the missing *bucket-level* cases (probes, estimate,
`TryConsumeAsMuchAsPossible`, blocking `Consume`) to the existing
`tck.Run(t, func() *core.ProxyManager)` signature. This adds a differently
shaped entry point on a different boundary, and needs `export_test.go` work in
two backends that row 7 does not. Either can land first; neither blocks the
other. New roadmap row 20, tier 1, same reason row 7 is tier 1.

**Out of scope**
- Cancellation arriving *mid-flight*, after the request is on the wire. Not
  deterministically testable against a local container — the same race that
  kept AC-50 out of the k6 gate — and a flaky conformance case is worse than
  none. The case here is "already cancelled before the call", which is what
  proves the driver checks at all.
- Changing `CASStore` / `LockStore`. The contract already says `ctx`; this
  makes it enforced, not different.
- Any new backend. Four existing ones.

**AC id: AC-52**, assigned now. AC-49 to AC-51 are claimed by other in-flight
work, including on branches not yet merged — check `git ls-remote` before
assuming AC-53 is free for the next task.

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
  `Bucket.TryConsume` returns the error shape the ratified ruling names (see
  `docs/adr/0015-cancellation-is-not-a-store-failure.md` and its "Unresolved"
  section — do not write this case until that is settled) and leaves the token
  unspent.
