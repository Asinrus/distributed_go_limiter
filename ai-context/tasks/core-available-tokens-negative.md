# `AvailableTokens` can return a negative number

**Symptom:** one goroutine calls `AsBlocking().Consume(ctx, 5)` on an empty
bucket. While it waits, another goroutine's `AvailableTokens(ctx)` returns
`-5`. A token count is a count; the API has no reading below zero.

**Evidence** (confirmed against `origin/main`, commit `89b06ff`)
- `core/ops.go:75-77` — `availableAt` refills and returns `s.currentSize` raw.
  Line 77 is the unclamped return. (The function is `availableAt`, not
  `availableTokens`.)
- `core/ops.go:40` and `core/ops.go:55` — the two probe reject branches clamp
  the identical field with `max0(s.currentSize)`. Two of three reads clamp; one
  does not. `max0` already exists at `core/ops.go:10`.
- `core/ops.go:84,90` — `reserve` deducts `n` and lets the count go negative,
  commented "may go negative; future refills repay the reservation". This is
  how `BlockingBucket.Consume` holds a waiter's place in line, and it is the
  only way `currentSize` drops below zero: `tryConsume` guards with
  `if n <= s.currentSize` (`core/ops.go:20`).
- Both implementations route through the same read — `core/local.go:79` and
  `core/proxymanager.go:139` — so one clamp covers local and every backend.
- Distributed is not merely reachable, it is **persisted**: `reserve` reports
  `modified=true`, so `proxyBucket.reserveTokens`
  (`core/proxymanager.go:151-159`) writes the negative state through the
  executor, `core/codec.go:21` encodes it and `core/codec.go:35` decodes it
  with no validation. Every node that reads that key afterwards sees the
  negative until refills repay it. That is why the fix belongs at the read
  site rather than in what gets written.

**Checked and clear — not part of this task**
- `ConsumptionProbe.RemainingTokens` (`core/ops.go:32`) and
  `EstimationProbe.RemainingTokens` (`core/ops.go:51`) are the unclamped reads
  in the *accept* branches, which are gated by `n <= s.currentSize` with
  `n >= 1` enforced (`ErrNonPositiveTokens`). The result is `>= 0` in both.
  Confirmed, not assumed.
- `consumeAsMuch` (`core/ops.go:61-68`) is a third unclamped read, guarded by
  `if toConsume <= 0 { return 0, false }`. Also safe. It is not in the original
  report; it was checked because it reads the same field.
- `fullRefillNanos` (`core/state.go:162-166`) reads `currentSize` too, but
  returns a duration, not a count. A negative size sends it down the
  `delayNanosForConsume` branch, i.e. a longer wait — the honest answer for a
  bucket carrying a reservation. No leak.
- No existing test asserts a negative from `AvailableTokens`, so the clamp
  regresses nothing.

**The fix.** Clamp at `core/ops.go:77` with the helper two of its siblings
already use: return `max0(s.currentSize)`. One line, no new helper, no new
type, no change to the reservation mechanism.

**Out of scope: the internal ledger stays signed.** `currentSize` continuing to
go negative is the deliberate mechanism ported from bucket4j
(`core/state.go:6`, `core/ops.go:84`) and it is what makes a blocking waiter's
place in line survive across nodes. Replacing it with a never-negative encoding
— a GCRA-style "next permitted time" instead of a token count — is a codec
format change and belongs to roadmap row 17 (v0.2), where the multi-bandwidth
break already lives. **The owner's ruling ("a negative capacity is not
acceptable to expose — keep it as just a number") is satisfied by the clamp**;
the internal encoding is left untouched. Recorded here explicitly so the owner
can say if they meant the internal state as well, in which case this becomes a
row 17 input rather than a one-line fix.

**No ADR.** Nothing here is hard to reverse or surprising: it makes one read
agree with the two beside it, and the interesting half — that the internal
ledger stays signed — is already covered by ADR 0001's v1 scope plus row 17.
An ADR whose content is "we clamped a return value" fails the bar in
`docs/adr/0007-tasks-done-workflow.md`.

**Interaction, noted only — do not act on it.** If the reservation-release idea
under discussion lands (refunding a cancelled blocking reservation), it shrinks
how long a negative is observable but does not remove it: the count is negative
from the moment `reserve` deducts, before any cancellation exists. The clamp is
needed either way.

**AC id: AC-53**, assigned now. Checked against `origin/main` *and* every open
remote branch, not just `main` — AC-49 through AC-52 are claimed by in-flight
work that has not merged (AC-52 by PR #19). Roadmap row 21 for the same
reason: row 20 is taken by PR #19's row, unmerged at the time of writing.

**Done looks like:** `AvailableTokens` returns `0`, never a negative, on every
backend and on `LocalBucket`, while a blocking `Consume` still waits the same
amount of time — the reservation is unchanged, only its visibility is.

## Test scenarios

**Black-box (tester, Gate 2):** no new scenario and no black-box surface — no
example exposes `AvailableTokens` over HTTP; `examples/redis/main.go:33` serves
one handler that only calls `TryConsume`. `e2e/k6/run-sandbox.sh redis` exits 0
with the existing contract unchanged (5 accepted, 3 rejected; refill checks
pass) as a no-regression run.

**Unit / integration (Dev):**
- AC-53, the repro: on a `LocalBucket` drained to empty, one goroutine calls
  `AsBlocking().Consume(ctx, 5)`; while it is waiting, `AvailableTokens` from
  another goroutine returns `0`, never a negative. Run under `-race`.
- AC-53, the same on a distributed bucket via `NewInMemoryProxyManager`, which
  additionally covers the encode/decode round trip — the state written by
  `reserve` is read back through `core/codec.go:35` and still reports `0`.
- AC-53: the reservation itself is unaffected — the blocking `Consume` returns
  after the same wait as before the change, and the tokens it reserved are
  still deducted. A clamp that leaked into the ledger would show up here as a
  waiter served early or served twice.
- AC-53: a bucket that is merely empty (not reserved) still reports exactly
  `0`, and a partially filled bucket still reports its real count. The clamp
  must not become a floor that hides a wrong positive value.
- The existing AC-23 refill assertions (`core/local_test.go:97`) still pass
  unchanged.
