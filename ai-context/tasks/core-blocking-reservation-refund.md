# A cancelled blocking Consume keeps the tokens it reserved

**Symptom:** `AsBlocking().Consume(ctx, 5)` reserves 5 tokens, then sleeps. The
caller cancels. `Consume` returns `ctx.Err()` and the 5 tokens stay spent — for
a request that was never served. Under load, where cancellation is normal, the
limiter over-counts exactly when it matters.

**Evidence**
- `core/ops.go:84-92` — `reserve` deducts `n` and lets `currentSize` go
  negative (`core/ops.go:90`, "may go negative; future refills repay the
  reservation"). It reports `modified=true`, so the deduction is **written
  through the executor before the sleep starts** — `core/blocking.go:37` calls
  `reserveTokens`, which on the distributed path is
  `core/proxymanager.go:151-159`. Every node sees the deduction.
- `core/blocking.go:48-56` — the timer/`ctx.Done()` select. The cancel branch
  (`core/blocking.go:54-55`) returns `ctx.Err()` and does nothing else.
- `core/blocking.go:22-23` — the godoc calls this intended: "ctx cancelled
  while waiting -> ctx.Err() (the reservation stands and is repaid by refill)".
  **Under the owner's ruling that sentence is now wrong** and this task
  rewrites it.

"Repaid by refill" is true and beside the point: the bucket recovers, but the
tokens were charged to nobody, so every other caller was rate-limited for a
request that did not happen.

**In scope**
- On the cancel branch only, run a compensating release of `n` **through the
  same executor** — a `releaseTokens` sibling of `reserveTokens` on the
  `reserver` interface (`core/bucket.go:51-53`), with the arithmetic as one op
  in `core/ops.go` beside `reserve`. Same reason as every other op: written
  once, and every backend inherits it. `LocalBucket` (`core/local.go:89`) and
  `proxyBucket` (`core/proxymanager.go:151`) both implement it.
- **The cancelled context cannot carry the write.** Use
  `context.WithoutCancel(ctx)` plus a short timeout of its own. (Note: this
  does *not* depend on the Go 1.23 pin, roadmap row 1 —
  `context.WithoutCancel` landed in Go 1.21, below that floor, and every
  `go.mod` here currently reads `go 1.26.5`. The dependency named when this was
  scoped does not exist; the feature is available before and after row 1.)
- **Clamp the release at capacity.** Refills may have run while the caller
  waited; adding `n` back unconditionally would inflate the bucket above its
  own capacity and hand out tokens that were never earned.
- **Race discipline: if the timer already fired, the consumption succeeded.**
  Go's `select` picks randomly when both cases are ready, so a `Consume` whose
  wait elapsed at the moment of cancellation must return `nil` and release
  nothing. Re-check the timer before releasing; releasing a completed
  consumption is a double-spend in the caller's favour, which is worse than the
  bug being fixed.
- Rewrite `core/blocking.go:22-23`'s godoc to describe the new behaviour and
  the failure case below.

**The error shape, decided — do not reopen.** On the cancel branch the release
either succeeds or it does not, and the owner's ruling is *best-effort but
reported*:

- **Release succeeded:** return the bare `ctx.Err()`, unchanged from today and
  consistent with `docs/adr/0015-cancellation-is-not-a-store-failure.md`.
- **Release failed:** return `fmt.Errorf("%w: %w", ctx.Err(), ErrReservationNotReleased)`
  with a new exported sentinel `ErrReservationNotReleased`. Both
  `errors.Is(err, context.Canceled)` and
  `errors.Is(err, core.ErrReservationNotReleased)` then hold — the double-`%w`
  pattern `storeErr` (`core/command.go:17-21`) already uses, not a new idiom.
- **`ErrReservationNotReleased` must not wrap `ErrStore`**, and the underlying
  store error is dropped. Both follow from ADR 0015: on a done context the
  context error is the one a caller acts on, and tagging this as a store
  failure would route a departed client into the fail-open branch — precisely
  what that ADR rejected. Dropping the cause is the same trade-off ADR 0015
  already ratified for `storeErr`, so it is not a new decision.

**Known ceiling — name it in a comment, do not solve it.** A CAS that commits
on the wire but loses its reply to the cancellation still over-counts by `n`:
`core` cannot tell "did not apply" from "applied, reply lost", and neither can
any of the four backends. Fixing that needs idempotency keys in the stored
state, which is a codec format change (roadmap row 17). Best-effort means
best-effort; the comment should say what would justify revisiting it.

**Out of scope**
- Refunding anything on the non-blocking path. `TryConsume` either takes the
  tokens and returns immediately or takes nothing; there is no window.
- The `!ok` and `wait <= 0` early returns in `Consume`
  (`core/blocking.go:42-46`): nothing was reserved in the first case and the
  consumption is already complete in the second.
- Making the reservation itself never-negative. That is the codec question in
  row 17, and it is explicitly not this task — see AC-53 (negative
  `AvailableTokens`, filed on its own branch).

**Interaction with AC-53.** The refund shortens how long a negative
`currentSize` is observable; it does not remove it. The count is negative from
the moment `reserve` deducts, before any cancellation exists, and it stays
negative for the whole normal (uncancelled) wait. AC-53's clamp at
`core/ops.go:77` is needed regardless, and neither task blocks the other.

**AC id: AC-54**, checked against `origin/main` and every open remote branch —
AC-49 to AC-53 are claimed by in-flight work, some of it unmerged. Roadmap row
22: row 20 is the deferred vendor-conformance guard and row 21 is AC-53, both
on unmerged branches.

**Done looks like:** a cancelled `Consume` leaves the bucket's token count
where it would have been had the call never happened (capacity permitting), a
caller can tell the difference between "cancelled, tokens returned" and
"cancelled, tokens still held", and `core/blocking.go`'s godoc says what the
code does.

## Test scenarios

**Black-box (tester, Gate 2):** no new scenario and no black-box surface — no
example calls `AsBlocking()`; the handlers use `TryConsume`
(`examples/redis/main.go:40`). `e2e/k6/run-sandbox.sh redis` exits 0 with the
existing contract unchanged (5 accepted, 3 rejected; refill checks pass) as a
no-regression run.

**Unit / integration (Dev):**
- AC-54, the core case: on a bucket drained to empty, `Consume(ctx, 5)` is
  cancelled mid-wait; afterwards `AvailableTokens` reports the same count as a
  run where `Consume` was never called at all. Assert the count, not just the
  error.
- AC-54 on a distributed bucket via `NewInMemoryProxyManager`, so the release
  is proven to travel through the executor and the codec rather than only
  through `LocalBucket`'s in-memory state.
- AC-54, the clamp: cancel a reservation after enough simulated time that
  refill has already restored the bucket. The release must leave the count at
  capacity, never above it. Drive this on the injected clock (`WithClock`,
  `core/proxymanager.go:26`), not on wall time.
- AC-54, the race: a `Consume` whose timer fires at the moment of cancellation
  returns `nil` and releases nothing — the tokens stay consumed exactly once.
  This is the case that fails as a double-spend if the timer re-check is
  missing, so it needs to be deterministic rather than timing-dependent.
- AC-54, the reported failure: with a store rigged to fail the release, the
  returned error satisfies both `errors.Is(err, context.Canceled)` and
  `errors.Is(err, core.ErrReservationNotReleased)`, and **not**
  `errors.Is(err, core.ErrStore)`. All three directions asserted.
- AC-54: a deadline that expires mid-wait behaves the same as an explicit
  cancel, with `context.DeadlineExceeded` in place of `context.Canceled`.
- Unchanged behaviour: an uncancelled `Consume` still waits the same duration
  and still consumes exactly `n`. The existing AC-32 cancel test
  (`core/blocking_test.go:45-60`) asserts only the error and the promptness of
  the return, never that the reservation stands, so it passes untouched — no
  test currently encodes the behaviour being changed.
