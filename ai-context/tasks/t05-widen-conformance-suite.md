# T05 — Widen the conformance suite past four cases

**Blocked by:** T03 and T04. T03 because the suite will assert
`AvailableTokens` and a cancelled blocking `Consume`, and both change under
AC-53/AC-54 — writing the cases first means writing them twice. T04 because
half of what is being added needs elapsed time, and without an injected clock
each new case costs a real sleep on four containers.

**For:** Dev. Roadmap row 7.

## Why

`core/tck/tck.go:33-38` runs exactly four cases: AC-41 key isolation, AC-42
consistency under contention, AC-43 atomic init, AC-44 removal. Meanwhile
`README.md` advertises provider-swap (AC-46) as the project's central claim,
and `ai-context/README.md` says the same suite passes against every backend.

Four cases do not carry that claim. `TryConsumeAndReturnRemaining` (AC-17),
`EstimateAbilityToConsume` (AC-18), `TryConsumeAsMuchAsPossible` (AC-19) and
blocking `Consume` (AC-31) are exercised **only** against `LocalBucket` and the
in-memory store (`core/local_test.go:114,134,153`, `core/blocking_test.go:12`).
A backend that encoded a probe field wrong, or that lost a reservation write,
would pass the whole suite today. Those are exactly the operations where a
distributed path can differ from a local one — they read more state, and one of
them writes a negative count through the codec.

Note this ticket is named against `core/tck` because T02 has not landed yet;
by the time it is worked, the package is `core/storetest`.

## New AC ids

**AC-56 … AC-59, assigned here.** Verified free by scanning every ref, not just
`main`: `git branch -a` then grepping `AC-[0-9]+` across every branch's tree.
The highest id claimed anywhere is **AC-55** (the CAS retry budget,
`core/casretrybudget_test.go:10`) — AC-49 through AC-55 were each taken by
in-flight work before it merged, which is why the check has to cover branches.

| id | Criterion | Local counterpart |
|----|-----------|-------------------|
| AC-56 | `TryConsumeAndReturnRemaining` reports the same probe through a real store as it does locally — on accept *and* on reject, including `NanosToWaitForRefill` and `NanosToWaitForReset` | AC-17 |
| AC-57 | `EstimateAbilityToConsume` reports correctly through a real store and consumes nothing — a second identical estimate returns the same answer | AC-18 |
| AC-58 | `TryConsumeAsMuchAsPossible` drains to available-or-limit through a real store, and returns 0 without a write on an empty bucket | AC-19 |
| AC-59 | Blocking `Consume` reserves through the store, waits, and succeeds; the reservation is visible to a second bucket handle on the same key while the wait is in progress | AC-31 |

Separate ids rather than reusing AC-17/18/19/31: the local criterion and the
through-a-store criterion are different claims, and the whole point of the
suite is that the second one is not implied by the first. This also matches how
AC-41 … AC-44 already relate to their local siblings.

## In scope

- Four new subtests in the suite's `Run`, named `AC56_…` … `AC59_…` to match
  the existing convention.
- Every new case drives time through the injected clock T04 makes reachable —
  **no wall-clock sleeps in the suite.** The suite runs on four containers in
  CI; a sleep is paid four times, every push.
- `slowConfig` (`core/tck/tck.go:27`) exists to make refill a non-event. AC-56,
  AC-58 and AC-59 all need refill to actually happen, so they need a second
  config helper rather than a mutated `slowConfig` — the existing four cases
  depend on refill staying negligible.
- AC-59 is the one that needs care: it must assert the reservation is
  **persisted and shared**, not just that `Consume` returned nil. A second
  `pm.Bucket` handle on the same key, reading mid-wait, is what distinguishes a
  real distributed reservation from a local one.

## Out of scope

- A cancelled blocking `Consume` through a real backend (AC-54's refund). It
  belongs in the suite eventually, but T03 ships its own coverage first and
  adding it here would make T05 fail for T03's reasons.
- Vendor ctx-conformance (roadmap row 20) — un-deferred now that etcd is
  settled, but a separate row with its own file
  (`ai-context/tasks/tck-vendor-ctx-conformance.md`).
- Any change to the four existing cases.

## Test scenarios

**Black-box (tester, Gate 2):** no new scenario and no black-box surface — this
ticket adds test code only; no shipped behaviour changes. Run
`e2e/k6/run-sandbox.sh redis` once as a no-regression check (5 accepted, 3
rejected; refill checks pass; exit 0).

**Unit / integration (Dev):** the four criteria above, each run against
**every** backend — `redisstore`, `pgstore`, `mongostore`, `etcdstore` — and
against the in-memory manager via `core`'s AC-46 run. Concretely:

- **AC-56 accept:** consume from a partially drained bucket; `Consumed` true,
  `RemainingTokens` exact, `NanosToWaitForReset` matches the local answer for
  the same state. **AC-56 reject:** ask for more than is available;
  `Consumed` false, `RemainingTokens` is the real count and never negative
  (AC-53's clamp, now observed through the codec), `NanosToWaitForRefill` is
  the wait that actually works — advance the injected clock by exactly it and
  the same consume succeeds. That round trip is the assertion that catches a
  backend mangling the field.
- **AC-57:** estimate on a bucket that cannot satisfy the request returns
  `CanBeConsumed` false with a wait; estimate again and get the identical
  answer; then `AvailableTokens` is unchanged from before either call. The
  "consumes nothing" half is what a store with an over-eager write breaks.
- **AC-58:** on a bucket with 6 of 10, an unlimited drain returns 6 and leaves
  0; a limited drain of 3 on a full bucket returns 3 and leaves 7; a drain on
  an empty bucket returns 0. Assert the count *and* that the empty case leaves
  the bucket at 0 — a backend that writes on a no-op is a wasted round trip and
  a lost CAS race under contention.
- **AC-59:** drain the bucket, start `Consume(ctx, n)` on one handle, and from a
  second handle on the same key observe that the tokens are already committed
  before the wait ends; let the injected clock advance and confirm `Consume`
  returns nil. Then a third read shows the bucket settled at the arithmetically
  correct count, not double-charged.
- The existing AC-41 … AC-44 cases pass unchanged on all four backends, and
  total suite wall-clock time does not grow — if it does, a sleep got in.

**Done looks like:** the suite covers every operation on the `Bucket`
interface against a real store, `README.md`'s provider-swap claim is backed by
the suite rather than by four cases, and the run is no slower than before.
