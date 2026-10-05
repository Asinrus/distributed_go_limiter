# T03 — Reservation semantics: AC-53 and AC-54 ship together

**Blocked by:** T01 — both AC-53 and AC-54 are deliberate deviations from
bucket4j, and `docs/adr/0018-deliberate-deviations-from-bucket4j.md` is what
keeps a future contributor from reverting them. The ADR lands first.

**For:** Dev.

## This is a ticket, not a third spec

One spec per AC id, and both already exist. Do not rewrite them here:

- **AC-53** — `ai-context/tasks/core-available-tokens-negative.md`. The whole
  change is one line: `core/ops.go:77` returns `max0(s.currentSize)`, reusing
  the helper already at `core/ops.go:10` and already used by the two probe
  reject branches at `core/ops.go:40` and `core/ops.go:55`. That file also
  carries the audit of every other read of `currentSize` (`core/ops.go:32,51`,
  `consumeAsMuch`, `fullRefillNanos`) and the finding that each is already
  safe — that audit is the reason this stays one line, so read it rather than
  redoing it.
- **AC-54** — `ai-context/tasks/core-blocking-reservation-refund.md`. Its "In
  scope", "The error shape, decided", "Known ceiling" and "Test scenarios"
  sections all stand as written and are the spec.

**One premise in the AC-54 file has changed and the file has not been edited to
say so.** It reads as a bug fix — "the godoc calls this intended … under the
owner's ruling that sentence is now wrong". We now know upstream **deliberately**
does not refund: `BlockingBucket.java:47-51` documents the non-refund in the
method contract and offers `Bucket#addTokens(long)` as the escape hatch,
`AbstractBucket.java:198-204` is a bare catch-and-rethrow with no `finally`, and
`BlockingConsumeSpecification.groovy:116-128` pins it with a test. So this is a
**chosen deviation**, not a defect being repaired. Nothing in the mechanics
changes; what changes is the justification, and the justification now lives in
ADR 0018 rather than in "the godoc was wrong". Reference the ADR from the code
comment, not the ruling.

## Added by this ticket

- **Rewrite the godoc at `core/blocking.go:22-23`.** It currently reads
  "ctx cancelled while waiting-> ctx.Err() (the reservation stands and is
  repaid by refill)" — a correct description of behaviour this ticket removes.
  The replacement must state: the reservation is released, the error is the
  bare context error when the release succeeds, and the error additionally
  satisfies `errors.Is(err, ErrReservationNotReleased)` when it does not. This
  is the single most likely thing to be left stale, because the code compiles
  either way.
- Ship both ACs in one change. They touch adjacent lines in `core/ops.go` and
  the same reservation mechanism, and shipping them apart means two rounds of
  review on the same paragraph of ADR 0018. They are independent in behaviour —
  AC-53's clamp is needed whether or not the refund exists, since the count is
  negative from the moment `reserve` deducts — so either order works inside the
  change.

## Out of scope

- Making the internal ledger never-negative. ADR 0018 rejects it explicitly:
  it is a codec format change and belongs with roadmap row 17 (v0.2).
- Refunding on the non-blocking path — `TryConsume` has no window.

## Test scenarios

**Black-box (tester, Gate 2):** no new scenario and no black-box surface — no
example calls `AsBlocking()` or exposes `AvailableTokens` over HTTP; the
handlers call `TryConsume` only. Run `e2e/k6/run-sandbox.sh redis` as a
no-regression check: 5 accepted, 3 rejected, refill checks pass, exit 0.

**Unit / integration (Dev):** the AC-53 list in
`core-available-tokens-negative.md` and the AC-54 list in
`core-blocking-reservation-refund.md`, in full — both are already written from
the outside in and neither is superseded. Two additions:

- **The godoc matches the code.** A test that cancels a blocking `Consume` on
  an in-memory distributed bucket and asserts, in the same test, both of the
  things `core/blocking.go`'s doc comment now promises: the returned error and
  the restored token count. The point is that the assertion sits next to the
  claim, so a future behaviour change breaks the test that guards the sentence.
- **AC-53 and AC-54 together, not just separately:** a bucket drained to empty,
  a blocking `Consume(ctx, 5)` cancelled mid-wait, and `AvailableTokens` read
  at three moments — before the reservation, during the wait, and after the
  cancel. Expected `0`, `0`, then the pre-reservation count. The middle reading
  is AC-53 (the clamp hides a genuinely negative ledger) and the last is AC-54
  (the refund restores it); run in one test they also prove the clamp is not
  silently doing the refund's job.

**Done looks like:** `AvailableTokens` never returns a negative on any backend
or on `LocalBucket`; a cancelled `Consume` leaves the count where it would have
been had the call never happened, capacity permitting; a caller can distinguish
"cancelled, tokens returned" from "cancelled, tokens still held"; and
`core/blocking.go`'s godoc describes the code that is there.
