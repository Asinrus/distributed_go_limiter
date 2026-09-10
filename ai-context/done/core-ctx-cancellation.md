# A cancelled context still consumes a token

**Symptom:** call `TryConsume` with an already-cancelled context on an
in-memory distributed bucket. It returns `(true, nil)` and the token is gone.
The API says the opposite.

**Evidence**
- `core/bucket.go:29` — "Distributed implementations honour ctx and may return
  ErrStore". That is the contract being broken.
- `core/memstore.go:43` — `GetState(_ context.Context, ...)`; the context is
  discarded. Same at `core/memstore.go:53` and `core/memstore.go:76`.
- `core/command.go:44-45` — `casRunner.execute` forwards ctx to the store and
  never inspects it, so when the store ignores ctx nothing else catches it.
  `core/command.go:84` — same for `lockRunner.execute`.
- `core/blocking.go:54-55` honours ctx correctly (AC-32). The gap is the
  distributed store path, not the local blocking wait — do not "fix" the part
  that already works.

Real backends partly mask this: the Redis, pgx, Mongo and etcd drivers all
honour a cancelled context and fail the round trip. But the honouring is then a
property of each driver rather than of this library, and cancellation observed
*between* a successful read and the swap is still unhandled everywhere.

**Why it matters under load.** Cancellation is not an edge case in an HTTP
server: `r.Context()` is cancelled the moment the client disconnects, and
clients disconnect most under exactly the load where the limiter matters. Every
example already passes `r.Context()` straight through
(`examples/redis/main.go:40`). Tokens consumed on behalf of requests nobody is
waiting for are tokens taken from requests that are — a limiter that
over-counts under load rejects legitimate traffic it should have served.

## The error shape is decided: bare context error

Ratified as `docs/adr/0015-cancellation-is-not-a-store-failure.md`. Dev does
not reopen it. In short: a cancelled or expired context yields
`context.Canceled` / `context.DeadlineExceeded` **unwrapped**, and
`errors.Is(err, core.ErrStore)` is **false** for it. `ErrStore` is the
fail-open/fail-closed switch for a backend outage (ADR 0002); a client that
hung up is not an outage.

That makes `storeErr` (`core/command.go:17`) the single choke point, not just
the new guards: it wraps every error crossing the store boundary, so a driver
that aborts on a cancelled context produces `ErrStore` today while a bare guard
would produce a context error — same cause, two shapes, decided by a race.
`storeErr` must return the context's error unwrapped whenever the context is
done, discarding the underlying error. Both facts Dev needs from that:

- A genuine store error that happens to coincide with a cancellation is
  reported as the cancellation. Intended; do not add a second error shape to
  preserve it.
- The decode-failure guard at `core/command.go:100-102` runs **before**
  `storeErr` in `lockRunner.execute` and must keep doing so. A corrupt-state
  decode error is neither a store failure nor a cancellation.

**In scope**
- `casRunner.execute` and `lockRunner.execute` check the context before doing
  work, and `casRunner.execute` re-checks before each retry, returning the
  context's error. Putting the check in the executor rather than in each store
  is the point: it makes the behavior uniform across every backend instead of
  dependent on how diligent a driver is.
- `storeErr` becomes context-aware, per the ruling above. `remove` on both
  runners (`core/command.go:74`, `core/command.go:106`) routes through it and
  inherits the behavior for free — that is why the choke point is there and not
  duplicated at each call site.
- `core/bucket.go:19-24` — "cancellation" comes out of `ErrStore`'s enumerated
  causes; the doc names the context errors as a separate outcome instead.
- **Hygiene, not the fix:** `MemStore` returns the context error from
  `GetState`, `CompareAndSwap` and `Remove` (`core/memstore.go:43,53,76`). It
  is in-process — there is no round trip to abort, and the executor guard
  already makes the observable behavior correct without it. It is in scope only
  because `MemStore` is the reference `CASStore` that a third-party backend
  author copies, and a reference implementation that discards its context
  teaches the wrong shape. Three one-line guards. If it is dropped, nothing
  regresses; if it is kept, no AC-50 test may depend on it (see Test scenarios).

**Interaction with AC-51.** The `ErrStore` criterion is ratified separately in
`ai-context/tasks/q9-errstore-contract-ac.md` (open PR #11) and this task does
not restate or renumber it. But its clause 1 — "every failure originating at
the store boundary surfaces as `errors.Is(err, core.ErrStore)`" — is written
without a cancellation exception, so ADR 0015 makes cancellation a **carve-out
from AC-51 clause 1**, not a sixth clause. Whoever merges second reconciles the
wording; nobody assigns a new id for it.

**Dependency, deliberately not absorbed.** The right long-term home for the
cancellation assertion is the TCK, so every backend is held to it and not just
`MemStore`. That means widening `core/tck/tck.go:35-38`, which is its own
queued item (row 7 of `ai-context/tasks/next-roadmap.md`, tier 1 — the owner's
ruling that cheap pluggability is the product makes a four-case TCK the weak
link). This task fixes the behavior and covers it in `core`'s own tests; adding
the TCK case is row 7's job. Do not silently rewrite the TCK here.

**Out of scope**
- Backoff and jitter in the retry loop — that is
  `ai-context/tasks/core-cas-retry-hardening.md`, and it stays a separate task.
  Both touch `core/command.go:45`, and the merge was considered and rejected:
  this task is a behavioral contract with an AC and a five-line diff, that one
  is a pacing design with no AC and an injectable-timer question still open.
  Merging makes a shippable correctness fix hostage to a tuning debate in
  review. **This one lands first**, and it leaves that one no open question: a
  backoff sleep must be interruptible by `ctx.Done()` and must return the same
  bare context error ADR 0015 fixes here.
- Rolling back a consumption that a later cancellation would have prevented.
  The check is "do not start work that is already pointless", not a transaction.

**AC id: AC-50** (cancellation on the distributed path — AC-32 covers the local
blocking wait only), assigned now, not at pickup. AC-49 is
`core-initial-tokens-validation.md`, AC-51 the `ErrStore` contract, AC-52 the
vendor conformance sibling (`ai-context/tasks/vendor-ctx-conformance.md`).

**Done looks like:** an already-cancelled context never consumes a token
through any backend, the error a caller sees is the bare context error and is
distinguishable from a store outage in both directions, `core/bucket.go`'s
contract and the code agree, and the `ErrStore` tests carry an AC id.

## Test scenarios

**Black-box (tester, Gate 2):** `e2e/k6/run-sandbox.sh redis` exits 0 with the
existing contract unchanged (5 accepted, 3 rejected; refill checks pass). No
new scenario — but not because there is no surface. There is one: a k6
per-request `timeout` short enough to abort the request client-side cancels
`r.Context()` in the example handler (`examples/redis/main.go:40`), and "N
aborted requests, then 5 normal ones all accepted" would be visible over HTTP
with no new endpoint. It is left out because whether the cancellation lands
before or after the store round-trip is a race the scenario cannot control —
sub-millisecond against a local container — so the check would fail
intermittently even on correct code. A flaky gate is worse than none. Revisit
if the sandbox ever gains a controllable delay on the store path.

**Unit / integration (Dev):**
- **Use a store that ignores its context** for the AC-50 cases — the stub in
  `core/storeerr_test.go:27-34` already is one. If the test drives `MemStore`
  after the hygiene item lands, it passes even with the executor guard missing,
  and then it proves the store rather than the fix.
- AC-50: `TryConsume` with an already-cancelled context returns an error, and a
  subsequent call with a live context finds the bucket still full — the token
  was not spent.
- AC-50: the same for `TryConsumeAndReturnRemaining`,
  `EstimateAbilityToConsume` and `TryConsumeAsMuchAsPossible`. A read-only
  operation on a cancelled context must fail too; the executor is the choke
  point, so one guard covers all of them and the tests should prove that.
- AC-50: an expired deadline behaves the same as an explicit cancellation.
- AC-50: cancellation arriving mid-retry stops the CAS loop rather than running
  out the remaining attempts.
- AC-50: the lock path (`lockRunner`) refuses an already-cancelled context too,
  not just the CAS path.
- The error is `errors.Is(err, context.Canceled)` and **not**
  `errors.Is(err, core.ErrStore)` — both directions asserted, on both runners.
- A store that fails with its own error on a *live* context still
  returns `ErrStore` with the backend error reachable. The existing eight cases
  in `core/storeerr_test.go` cover this already; they belong to AC-51 and are
  not this task's to rename.
- AC-50: a store that returns a real backend error while the context is
  cancelled reports the cancellation, not `ErrStore` — this is the choke-point
  behavior and the one place the ruling is visible as a deliberate choice.
- Hygiene item only, if kept: `MemStore` returns the context error directly
  from `GetState`, `CompareAndSwap` and `Remove`. Assert it against the store
  directly, never through a `Bucket` — a test that reaches it through the
  bucket is testing the executor guard again.
