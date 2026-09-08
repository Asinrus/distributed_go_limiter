# The CAS retry loop is a hot spin with no backoff

**Symptom:** on contention, `casRunner.execute` re-reads and re-swaps up to 100
times as fast as the network allows. Every loser retries immediately, and every
retry is two more round trips to the store.

**Evidence**
- `core/command.go:24` — `maxCASRetries = 100`.
- `core/command.go:45` — `for i := 0; i < maxCASRetries; i++` with no delay
  between iterations and no jitter.
- `core/command.go:70` — after 100 attempts the caller gets
  `ErrCASRetriesExhausted`, which wraps `ErrStore`.

**Why it matters under load.** This is the classic thundering herd. Hot keys
are the normal case for a rate limiter, not the exception: one popular API key
or one aggressive IP is exactly the traffic the limiter exists to shape. When
K callers contend on one key they all lose together, all retry together, and
the retries themselves are what keep the window contended — load on the store
grows with the square of the contention rather than with the request rate.
100 immediate attempts is also, in the worst case, 200 round trips charged to a
single HTTP request that was going to be rejected anyway.

The `pgstore` lock path has no equivalent problem: it blocks on a row lock
rather than spinning. This is a CAS-path defect (`redisstore`, `mongostore`,
`etcdstore`, `MemStore`).

**In scope**
- Backoff between CAS attempts, with jitter. Jitter is the part that matters —
  a fixed backoff re-synchronizes the herd instead of dispersing it.
- Justify `maxCASRetries = 100` or change it. With backoff, 100 attempts also
  becomes a latency budget, so the two numbers cannot be picked independently.
  Whatever the bound is, exhausting it must still return
  `ErrCASRetriesExhausted` wrapping `ErrStore` — that contract is relied on.
- The delay must be injectable so tests do not sleep. `core` already has the
  pattern (`core.WithClock`, `core/proxymanager.go:26`); do not invent a second
  one.

**Out of scope**
- **The `ctx.Done()` check inside the loop belongs to
  `ai-context/tasks/core-ctx-cancellation.md`, not here.** Both tasks touch
  `core/command.go:45`; that one owns cancellation, this one owns pacing. The
  merge was considered and rejected: that task is a behavioral contract with
  two AC ids and a five-line diff, this one is a pacing design with no AC and
  an open question about the injectable timer, and merging them makes a
  shippable correctness fix hostage to a tuning debate in review. The ctx task
  lands first — a backoff sleep that ignores cancellation is worse than no
  backoff — and it leaves this one **no open question about the shared line**:
  the sleep must be interruptible by `ctx.Done()`, and on interruption it
  returns the bare `context.Canceled` / `context.DeadlineExceeded`, not
  `ErrStore` (`docs/adr/0015-cancellation-is-not-a-store-failure.md`). Expect a
  trivial rebase on the loop header, not a design conflict.
- Changing `CASStore` or any backend. This is `core` only.
- Adaptive or per-key backoff tuning, and any metric or log of retry counts
  (verbose diagnostics are deferred out of v1, ADR 0001).

**AC id: none.** Ruled, so it is not reopened at pickup: retry *pacing* is an
implementation property, not a behavioral criterion — nothing observable to a
caller changes, which is why the black-box note below says a green sandbox
proves nothing here. The one part that is behavior, "exhaustion returns
`ErrCASRetriesExhausted` wrapping `ErrStore`", predates this task and belongs
to the `ErrStore` criterion, now **AC-51** — ratified in
`ai-context/tasks/q9-errstore-contract-ac.md`; do not assign it a second id
here. AC-49 to AC-52 are taken by other tasks.

This ruling still stands for the pacing. It does **not** cover the
caller-configurable budget added after this file was gated — that is a public
option with an observable contract and it carries **AC-55**, in
`ai-context/tasks/core-cas-retry-budget-option.md`.

**Done looks like:** concurrent callers on one key no longer retry in lockstep,
the retry bound is a stated latency budget rather than an arbitrary number, the
exhaustion error is unchanged, and no test sleeps in wall-clock time to prove
it.

## Test scenarios

**Black-box (tester, Gate 2):** `e2e/k6/run-sandbox.sh redis` exits 0 with the
existing contract unchanged (5 accepted, 3 rejected; refill checks pass). The
sandbox drives a single key from one VU, so it does not generate contention —
it is a no-regression check, and the contention proof is Dev's. Do not read a
green sandbox as evidence that backoff works.

**Unit / integration (Dev):**
- Under concurrent consumption of one key by many goroutines against
  `MemStore`, the total number of tokens granted still equals capacity exactly.
  Backoff must not change the outcome, only the pacing — this is the existing
  AC-42 consistency property and it must not regress.
- A store rigged to always lose the swap returns `ErrCASRetriesExhausted`,
  `errors.Is(err, core.ErrStore)` holds, and the number of attempts made equals
  the stated bound — no more, no fewer.
- Successive attempts are spaced by a growing, non-identical delay, asserted
  through the injected time source rather than by sleeping.
- The single-caller uncontended path makes exactly one attempt and incurs no
  delay. A limiter that pays backoff on the common case has traded the wrong
  way.
