# T06 — pgstore's lock retry loop spins with no backoff and a budget measured to fail

**Blocked by:** T04. Both change how `pgstore.NewProxyManager` takes options,
and T04 is already in flight in that file.

**For:** Dev. **Queued as a follow-up — the owner chose not to build it this
round.** The design question it carried is now ruled, so it is ready to pick up
rather than blocked on an answer.

## The defect

`pgstore/pgstore.go:88` declares `const maxLockRetries = 16` and
`pgstore/pgstore.go:92` wraps `doOnce` in a tight `for i := 0; i < maxLockRetries; i++`.
No backoff, no jitter, and no `ctx.Done()` check between attempts. On lock
contention it retries against the database as fast as the round trip allows —
every losing writer in lockstep, which is the pattern that turns contention
into a thundering herd rather than damping it.

**The asymmetry is what makes this sharp.** `core` already solved exactly this
for the CAS path and the lock path never received it:

| | CAS path | pgstore lock path |
|---|---|---|
| pacing | `casBackoff` (`core/command.go`), full jitter, base 1ms, cap 32ms | none |
| ctx-interruptible sleep | `sleepCtx` (`core/command.go`) | n/a |
| budget | `defaultMaxCASRetries = 64`, configurable via `core.WithMaxCASRetries` | `maxLockRetries = 16`, fixed |

That 64 is not a guess. Its comment in `core/command.go` records the
measurement: **16 failed against mongostore with retries exhausted, and 32
flaked the same way on a repeat run.** pgstore's lock path uses that same 16,
with none of the pacing that would make a small budget survivable. The
`LockStore` side simply never got the PR #17 / AC-55 treatment.

**AC-60** for the fix. Verified free by grepping `AC-[0-9]+` across every ref
from `git branch -a`, not just `main`: the highest claimed anywhere is AC-59
(T05, `t05-widen-conformance-suite.md`).

## Two things checked and found clean — do not re-open them

Both were raised as candidate second defects. Neither survives contact with the
code, and that is worth recording so nobody re-derives it:

- **`Do`'s error wrapping at `pgstore/pgstore.go:95` does not violate ADR
  0015.** It wraps every error, `context.Canceled` included, in
  `pgstore: do %q: %w`. But `lockRunner.execute` returns through `storeErr`
  (`core/command.go`), which checks `ctx.Err()` **first** and returns the bare
  context error, discarding pgstore's wrapping entirely. A cancelled caller
  still gets the bare `ctx.Err()` that
  `docs/adr/0015-cancellation-is-not-a-store-failure.md` promises. This is
  exactly the "single choke point" that ADR is about, working as designed.
- **The exhaustion error at `pgstore/pgstore.go:102` gets the same error class
  as its CAS sibling.** It does not wrap `ErrStore` itself, but `storeErr`'s
  tail wraps it, so `errors.Is(err, core.ErrStore)` holds — matching
  `ErrCASRetriesExhausted`. No divergence for a caller choosing fail-open vs
  fail-closed.

One real but minor consequence stays in scope below: `lockRunner.execute`
guards the context **once**, before calling `Do`, so a cancellation arriving
mid-loop is noticed only when pgx fails the next query on the dead context. It
terminates — pgx honours the context — but by driver behaviour rather than by
this package's contract, and adding a ctx check between attempts costs one line
now that a sleep goes there anyway.

## Ruled: the retry lives in pgstore, as pgstore configuration

**Owner ruling:** *"we don't need to change the `Do` signature, we just need to
configure postgres proxy manager behaviour, that's all."* The hoisting analysis
below was confirmed, and the export-vs-duplicate question is **closed** — not by
picking a side, but by the reframe. If the budget and the backoff are pgstore
*configuration*, then pgstore owns the implementation and `core`'s deliberately
closed surface stays closed.

**Do not export `casBackoff` or `sleepCtx`.** pgstore implements its own pacing.

**The "measured constant" argument, which was the strongest case for exporting,
weakens under this reframe — deliberately recorded rather than quietly dropped.**
It says a value measured once must not be duplicated, and that holds for a shared
*hard-coded* constant. A configurable default is a different thing: the number is
a starting point a caller can move, not a tuning decision two code paths silently
share. What must not be lost is the *provenance*, so pgstore's defaults cite the
measurement in a comment rather than inheriting the symbol —
`defaultMaxCASRetries` and its doc comment (`core/command.go:34-47`): 16 exhausted
against mongostore, 32 flaked on a repeat run, 64 held across 2 redis, 3
mongostore and 3 etcd runs. A future reader must be able to see where pgstore's
numbers came from without a `git blame`.

**Hoisting into `core`'s lock executor stays out**, and this is the part that was
independently confirmed: **the `LockStore` contract has no channel for "not done,
retry me."** `Do` (`core/store.go`'s `LockStore`) returns a single `error`; "the
row was not there and a concurrent writer created it, so try again" is signalled
by `doOnce` returning `(false, nil)`, entirely inside pgstore. Hoisting means
adding a retry signal to `Do`'s signature — a contract change every future
`LockStore` pays for, to generalise a race specific to
`INSERT … ON CONFLICT DO NOTHING` followed by `SELECT … FOR UPDATE`
(`pgstore/pgstore.go:100` states the condition). A `LockStore` built on advisory
locks or an external lock service has no such race and nothing to put in that
field. `Do` keeps its single-`error` signature.

**No ADR.** The decision that would have earned one was exporting from `core`;
"a backend configures its own retry behaviour" is neither surprising nor hard to
reverse, and it fails the bar in `docs/adr/0007-tasks-done-workflow.md`.

## The budget option

**Do not reuse `core.WithMaxCASRetries`.** The name lies — there is no
compare-and-swap on this path — and the two budgets are not the same quantity:
a CAS retry costs a round trip, a lock retry costs an aborted transaction. A
caller tuning one would silently move the other.

**The shape is the Architect's call when this is picked up, not the owner's** —
T04 made both routes available, so it has to be chosen rather than defaulted.

**Recommended: a backend-level `pgstore.WithMaxLockRetries`** (and a pacing knob
if one is wanted), alongside the existing `pgstore.WithTTL` and siblings — *not*
a forwarded `core.ProxyOption`. Three reasons, in order of weight:

- A **new** core option named for locks would put lock semantics into a
  vendor-agnostic type. That is the same objection as reusing
  `WithMaxCASRetries`, from the other direction: every `CASStore` backend would
  then carry a knob that does nothing, and `core` would name a strategy only one
  backend implements.
- It matches the ruling's own framing — "configure postgres proxy manager
  behaviour". `pgstore.NewProxyManager` already takes `pgstore.Option`, so this
  needs no new plumbing at all.
- It keeps the retry's configuration next to the retry's implementation, which
  is now the same package by the ruling above.

Whatever is chosen, the default moves off 16. Match the CAS path's default of
64 unless measurement says otherwise — the CAS measurement is evidence about
retry budgets under real contention, not about CAS specifically, and 16 is the
number that measurement rejected.

## Out of scope

- The `Do` error-wrapping and `ErrStore` questions above — checked, clean.
- Any change to `doOnce`, the SQL, or the lock strategy. ADR 0003 fixes
  `SELECT … FOR UPDATE` for v1.
- Backoff for `Remove`, which does not retry.

## Test scenarios

**Black-box (tester, Gate 2):** `e2e/k6/run-sandbox.sh postgres` must exit 0
with the existing contract intact (5 accepted, 3 rejected; refill checks pass).
Then the case this ticket exists for: the same sandbox run at concurrency high
enough to contend one key, where **the accept/reject counts must be exactly the
arithmetic answer, not merely close** — a retry budget exhausting under
contention shows up as 503s, and pacing must not change the outcome, only the
timing. Compare against the redis sandbox at the same concurrency; the two
backends must agree on the counts.

**Unit / integration (Dev):**
- **AC-60, the budget:** with N writers contending one key through
  `pgstore.NewProxyManager` against a real Postgres container, every operation
  completes — none returns "exceeded retries acquiring row lock". Pick N high
  enough that the current code fails this; a test that passes before the change
  is not a test of it.
- **AC-60, the pacing:** losing attempts sleep before retrying, and the delay is
  jittered rather than fixed. Assert the property that matters — two
  simultaneous losers do not retry at the same instant — not the exact
  durations, which are random by design.
- **AC-60, cancellation:** a context cancelled while the loop is between
  attempts returns promptly rather than after the remaining attempts, and the
  error is the bare `context.Canceled` — `errors.Is(err, context.Canceled)`
  true and `errors.Is(err, core.ErrStore)` **false**, per ADR 0015. This is the
  ctx-check-between-attempts half.
- **AC-60, exhaustion still terminates:** with the budget set to 1 against a
  key held by another transaction, `Do` gives up rather than blocking forever,
  and the error satisfies `errors.Is(err, core.ErrStore)` — the behaviour
  verified clean above must survive the change.
- **Configurability:** whichever option shape is chosen, an out-of-range value
  is rejected at construction rather than clamped, matching
  `core.WithMaxCASRetries`.
- The existing `pgstore` suite (AC-41 … AC-44, AC-45, AC-47, AC-48) passes
  unchanged.

**Done looks like:** contended writers on one Postgres key back off instead of
spinning, the budget is at least the 64 that measurement supports, a cancelled
caller gets out between attempts, and the pacing decision lives in exactly one
place in the repo.
