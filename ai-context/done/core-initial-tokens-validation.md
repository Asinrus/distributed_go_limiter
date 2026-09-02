# `WithInitialTokens` accepts values the bucket can never honour

**Symptom:** `NewBandwidth(5).WithInitialTokens(50)` builds a valid config. The
bucket starts with 50 tokens against a capacity of 5, serves a burst of 50, and
nothing ever corrects it. `WithInitialTokens(-1)` is accepted too.

**Evidence**
- `core/bandwidth.go:50` — `WithInitialTokens` stores `n` unconditionally.
- `core/bandwidth.go:59` — `valid()` checks `capacity > 0`, `refillTokens > 0`,
  and `refillPeriodNanos > 0`. It never looks at `initialTokens`.
- `core/state.go:25` — the initial `currentSize` is `bw.initialTokens`, taken
  as given.
- `core/state.go:61` — refill returns early when `current >= capacity`. That
  guard exists for force-add overflow, and it is also what makes the
  over-capacity start permanent: refill never clamps *down*, so the only way
  back under capacity is spending the excess.

**Why it matters under load.** This is a silent limit breach, not a crash. A
service configured "100 requests/minute, start warm at 500" gets exactly what
it asked for and enforces a limit five times the intended one, once, at every
cold start of every key. Under per-key limiting with high key churn — the
per-IP pattern the examples demonstrate — every new key is a fresh cold start,
so the breach is continuous rather than one-off. Nothing in the logs says so.

**In scope**
- Reject an invalid `initialTokens` at config validation time.
  `core/config.go:19` is the single choke point: `Config.valid()` is called by
  both `NewLocalBucket` (`core/local.go:27`) and `ProxyManager.Bucket`
  (`core/proxymanager.go:61`), so one guard in `Bandwidth.valid()` covers the
  local and distributed paths at once. Do not add per-call-site checks.
- The rejection surfaces as the existing `ErrInvalidConfig`; no new error value.
- Godoc on `WithInitialTokens` states the accepted range.

**Out of scope**
- Clamping instead of rejecting. A config that says 50 against a capacity of 5
  is a mistake, and silently serving 5 hides it — the same reasoning that made
  `Bucket` validate up front (`core/proxymanager.go:56-60`) rather than
  re-check per operation.
- Changing refill's `current >= capacity` early return. It is correct for the
  case it was written for, and reachable state above capacity is a separate
  concern (force-add) that this task does not touch.
- Any blanket "current size must be non-negative" invariant.
  `core/state.go:35` documents that consume deliberately drives the count
  negative for reservations; only the *initial* value is being constrained.

**AC id: AC-49**, assigned now rather than at pickup — two tier-1 tasks wanted a
new id and "the next free one" resolves to the same number in both if they are
picked up together. `core-ctx-cancellation.md` holds AC-50;
`core-cas-retry-hardening.md` gets none (see that file). Taken today: 11-19,
21-23, 31-32, 41-46, 47, 48 — there is no acceptance-criteria document to
register this in (see row 11 of `ai-context/tasks/next-roadmap.md`), so the id
lives in this file and in the test name, as AC-47 and AC-48 did.

**Done looks like:** an out-of-range `initialTokens` cannot produce a usable
`Bucket` through either constructor, the accepted range is documented, and the
whole suite is still green.

## Test scenarios

**Black-box (tester, Gate 2):** no black-box surface — no example calls
`WithInitialTokens`, and building a demo binary with a deliberately invalid
config only to prove the constructor refuses it is not worth a sandbox variant.
Gate 2 is satisfied by Gate 1 (`docs/adr/0013-architect-scopes-test-coverage.md`).
The tester should still confirm one sandbox run (`e2e/k6/run-sandbox.sh redis`)
is unchanged, since this touches shared config validation.

**Unit / integration (Dev):**
- `initialTokens` greater than capacity is rejected, through **both**
  `NewLocalBucket` and `ProxyManager.Bucket`, with `ErrInvalidConfig`.
- A negative `initialTokens` is rejected the same way.
- `initialTokens == capacity` (the default, `core/bandwidth.go:28`) and
  `initialTokens == 0` are both accepted — an empty starting bucket is a
  legitimate configuration and must not be caught by the new guard.
- A bucket built with a valid smaller `initialTokens` still refills up to
  capacity and no further, so the guard changed rejection only, not the math.
