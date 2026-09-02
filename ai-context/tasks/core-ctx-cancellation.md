# A cancelled context still consumes a token

**Symptom:** call `TryConsume` with an already-cancelled context on an
in-memory distributed bucket. It returns `(true, nil)` and the token is gone.
The API says the opposite.

**Evidence**
- `core/bucket.go:29` — "Distributed implementations honour ctx and may return
  ErrStore". That is the contract being broken.
- `core/memstore.go:43` — `GetState(_ context.Context, ...)`; the context is
  discarded. Same at `core/memstore.go:53` and `core/memstore.go:76`.
- `core/command.go:44-71` — `casRunner.execute` forwards ctx to the store and
  never inspects it, so when the store ignores ctx nothing else catches it.
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

**In scope**
- `casRunner.execute` and `lockRunner.execute` check the context before doing
  work and before each retry, returning the context's error. Putting the check
  in the executor rather than in each store is the point: it makes the
  behavior uniform across every backend instead of dependent on how diligent a
  driver is.
- `MemStore` honours the context in `GetState`, `CompareAndSwap` and `Remove`.
  It is the reference `CASStore` implementation and the model third-party
  backends copy; a reference implementation that ignores its context teaches
  the wrong thing.
- Decide and document what the returned error is — `context.Canceled` /
  `context.DeadlineExceeded` reaching the caller unwrapped, or wrapped in
  `ErrStore`. `core/bucket.go:20-24` currently lists cancellation as one of the
  things `ErrStore` covers, so leaving the bare context error visible is a
  documentation change too, not a free choice. Whichever way it goes, callers
  must be able to tell "cancelled" from "backend down" — that is the whole
  reason `ErrStore` exists (ADR 0002).

**Dependency, deliberately not absorbed.** The right long-term home for this
assertion is the TCK, so every backend is held to it and not just `MemStore`.
That means widening `core/tck/tck.go:35-38`, which is its own queued item
(row 7 of `ai-context/tasks/next-roadmap.md`, now tier 1 — the owner's ruling
that cheap pluggability is the product makes a four-case TCK the weak link).
This task fixes the behavior and
covers it in `core`'s own tests; adding the TCK case is row 7's job, and this
file is the reason row 7 should not slip. Do not silently rewrite the TCK here.

**Out of scope**
- Backoff and jitter in the retry loop — that is
  `ai-context/tasks/core-cas-retry-hardening.md`. Both tasks touch
  `core/command.go:45`; **this one lands first**, because a backoff sleep that
  ignores cancellation is worse than no backoff.
- Rolling back a consumption that a later cancellation would have prevented.
  The check is "do not start work that is already pointless", not a transaction.

**AC id: AC-50**, assigned now, not at pickup. Cancellation on the distributed
path has no criterion (AC-32 covers the local blocking wait only). AC-49 is
`core-initial-tokens-validation.md`; both were claiming "the next free id",
which collides the moment they are worked in parallel.

**Done looks like:** an already-cancelled context never consumes a token
through any backend, the error a caller sees is documented and distinguishable
from a store outage, and `core/bucket.go`'s contract and the code agree.

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
- `TryConsume` with an already-cancelled context returns an error, and a
  subsequent call with a live context finds the bucket still full — the token
  was not spent.
- The same for `TryConsumeAndReturnRemaining`, `EstimateAbilityToConsume` and
  `TryConsumeAsMuchAsPossible`. A read-only operation on a cancelled context
  must fail too; the executor is the choke point, so all of them should be
  covered by one guard and the tests should prove that.
- The error a caller sees satisfies whatever the ruling above decides, and is
  distinguishable from a genuine store failure — a test that asserts both
  directions, not just one.
- An expired deadline behaves the same as an explicit cancellation.
- `MemStore` returns the context error directly from `GetState`,
  `CompareAndSwap` and `Remove`, so a third-party store copying it inherits the
  behavior.
- Cancellation arriving mid-retry stops the loop rather than running out the
  remaining attempts.
