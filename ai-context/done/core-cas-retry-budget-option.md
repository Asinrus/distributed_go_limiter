# The CAS retry budget is caller-configurable (`WithMaxCASRetries`)

Follow-on to `ai-context/done/core-cas-retry-hardening.md`. That task set the
pacing (full jitter, capped at 32ms) and the default bound of 64 attempts, and
it was ruled **no AC** because pacing is not observable to a caller. The owner
then asked for the bound itself to be configurable, which added a public
option: `core/proxymanager.go:41`, `WithMaxCASRetries(n int) ProxyOption`.

That option *is* observable through the public API — how many store attempts a
caller pays for, and whether a bad value produces a usable `Bucket` — so it
gets a criterion of its own. **The no-AC ruling for the pacing is unchanged**;
this id covers the configurable budget only, not the jitter, the 32ms cap, or
the choice of 64 as the default.

**AC id: AC-55.** Checked against `origin/main` and every open remote branch:
the highest id in existence is AC-54
(`ai-context/tasks/core-blocking-reservation-refund.md:92`, on `main`), and
AC-49 to AC-54 are all claimed, some by unmerged work. AC-55 is the next free
one. As with AC-47 to AC-54 there is no acceptance-criteria document to
register it in (row 11 of `ai-context/tasks/next-roadmap.md`), so the id lives
in this file and in the test names.

**AC-55 — the caller controls the CAS retry budget, and a bad budget is
rejected up front.**
1. A budget of `n >= 1` set with `WithMaxCASRetries(n)` is honoured exactly: an
   operation on a permanently contended key makes `n` store attempts and `n-1`
   backoff delays, then fails with `ErrCASRetriesExhausted` wrapping
   `ErrStore` (AC-51). `n = 1` means one attempt and no delay.
2. `n < 1` is rejected by `ProxyManager.Bucket` with `ErrInvalidConfig`, and no
   `Bucket` is returned. It is not clamped, not a panic, and never an
   `ErrStore` — caller misuse is not a store failure.
3. Clause 2 holds on a lock-based manager too, even though a `LockStore`
   serializes rather than retries, so misuse is caught identically whichever
   backend the caller swapped in.
4. Unset, the budget is the default 64.

**Why reject rather than clamp** — the same reasoning already ratified for
`WithInitialTokens` (AC-49) and `Bandwidth.valid()`: a config that asks for 0
retries is a mistake, and silently serving 1 hides it. Validating in `Bucket`
means every `Bucket` that exists is usable, so no operation has to re-check.
Because this only applies existing precedent it earns **no ADR** — nothing
here is new direction (`docs/adr/0007-tasks-done-workflow.md`).

**Out of scope**
- Multi-bandwidth, per-key or adaptive budgets, and any metric of retry counts
  (deferred out of v1, ADR 0001).
- Making the jitter cap or the growth curve configurable. One knob, and it is
  the one the owner asked for; a second is unrequested surface.
- Re-litigating the default of 64 — measured, and owned by the pacing task.

**Done looks like:** a caller can trade latency against retry tolerance with
one option, a non-positive value cannot produce a usable `Bucket` on either
manager, and the exhaustion error is unchanged.

## Test scenarios

**Black-box (tester, Gate 2):** no black-box surface — no example or sandbox
binary calls `WithMaxCASRetries`, and the single-VU sandbox generates no
contention, so a budget change is invisible there
(`docs/adr/0013-architect-scopes-test-coverage.md`). The tester should still
confirm one `e2e/k6/run-sandbox.sh redis` run is unchanged, since the pacing
work this extends was gated on that run and the budget option landed after
that gate.

**Unit / integration (Dev):**
- AC-55 clause 1: against a store that always loses the swap, a budget of 3
  makes exactly 3 attempts and 2 backoffs and returns
  `ErrCASRetriesExhausted` — asserted against the injected sleep, not by
  sleeping, and compared against the default so a silent regression to 64
  fails.
- AC-55 clause 1, the boundary: a budget of 1 makes exactly one attempt and
  never sleeps.
- AC-55 clauses 2 and 3: budgets of 0 and -1 give `ErrInvalidConfig` and a nil
  `Bucket` from both `NewCASProxyManager` and `NewLockProxyManager`, and the
  error does not satisfy `errors.Is(err, ErrStore)`.

Covered by `core/casretrybudget_test.go`.
