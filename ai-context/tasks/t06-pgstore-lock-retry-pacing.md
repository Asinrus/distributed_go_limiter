# T06 — pgstore's lock retry loop spins with no backoff and a budget measured to fail

**Blocked by:** T04. Both change how `pgstore.NewProxyManager` takes options,
and T04 is already in flight in that file.

**For:** Dev, after the open question below is answered. Do not implement past
the ruling.

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

## The open question: where does the retry live?

`casBackoff` and `sleepCtx` are unexported in `core`, so pgstore cannot reuse
them today. Three options were considered.

**Option 3 — hoist the retry loop into `core`'s lock executor — is ruled out,
and this part is not an open question.** It is the option that matches the
project's stated principle (the algorithm lives once in `core`, every backend
inherits it), and it is still wrong here, for a concrete reason: **the
`LockStore` contract has no channel for "not done, retry me."**
`Do` (`core/store.go`'s `LockStore`) returns a single `error`; "the row was not
there and a concurrent writer created it, so try again" is signalled by
`doOnce` returning `(false, nil)`, entirely inside pgstore. Hoisting the loop
means adding a retry signal to `Do`'s signature — a contract change every
future `LockStore` pays for, to generalise a race that is specific to
`INSERT … ON CONFLICT DO NOTHING` followed by `SELECT … FOR UPDATE`
(`pgstore/pgstore.go:100` states the condition). A `LockStore` built on
advisory locks or an external lock service has no such race and nothing to put
in that field. Generalising a vendor-specific race into the core contract is
the opposite of what the principle asks for.

**Options 1 and 2 are an owner-level call, and I am not taking it.** The
recommendation is option 1; the reason it is not mine to rule is that it
changes `core`'s public API.

- **Option 1 — export the pacing from `core`** (one helper, not two: a single
  exported backoff-with-full-jitter that both the CAS loop and pgstore call).
  **Recommended.** The thing that must not be duplicated is not fifteen lines
  of code, it is a *tuning decision that was measured once* — base 1ms, cap
  32ms, full jitter rather than fixed. Two copies of a measured constant drift,
  and the next person to re-tune it will find one of them.
- **Option 2 — duplicate it in pgstore.** Keeps `core`'s surface closed, which
  is a real property: `core` has zero external runtime deps and a deliberately
  small API, and this would add retry-pacing helpers to it permanently for one
  caller outside itself.

**The trade-off in one line:** option 1 spends public API surface to keep a
measured constant in one place; option 2 spends a duplicated constant to keep
the API closed. Neither is reversible cheaply once released — an exported
symbol cannot be withdrawn, and a drifted constant is not noticed until it
misbehaves. If option 1 is chosen it should get an ADR; option 2 does not need
one.

## The budget option

**Do not reuse `core.WithMaxCASRetries`.** The name lies — there is no
compare-and-swap on this path — and the two budgets are not the same quantity:
a CAS retry costs a round trip, a lock retry costs an aborted transaction. A
caller tuning one would silently move the other.

**Recommended: a backend-level `pgstore.WithMaxLockRetries`**, alongside the
existing `pgstore.WithTTL` and siblings, not a new `core.ProxyOption`. The
retry lives in pgstore; a core option that exactly one backend honours is worse
than a backend option, because every other backend then has a knob that does
nothing.

**Why this has to be answered now rather than defaulted:** T04 makes
`core.ProxyOption` reachable from `pgstore.NewProxyManager`. Before T04 a core
option was not plumbable and the question answered itself; after T04 both
shapes work, so picking by default picks silently.

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
