# Where this port deliberately deviates from bucket4j

`CLAUDE.md` opens by calling this a port of bucket4j, and `core/state.go:6`
calls the state a direct reduction of `BucketState64BitsInteger`. That framing
is load-bearing: when the Go code and the Java code disagree, the default
assumption — correctly — is that the Go code is wrong. This ADR names the two
places where the disagreement is a decision, so that a future contributor who
reads the upstream source does not "fix" them back out.

Both deviations were checked against upstream at the source, not from memory.
Both are about the *observable API*, not the algorithm: the token math in
`core/state.go` stays a faithful reduction, and nothing here changes what a
bucket lets through.

## 1. No read exposes a negative token count (AC-53)

**Upstream exposes negatives, unclamped.** `BucketState64BitsInteger`'s
`getAvailableTokens()` (`BucketState64BitsInteger.java:309-316`) takes the
minimum across bandwidths and returns it with no floor; `EstimationProbe` and a
rejected `ConsumptionProbe` pass that raw value straight through. A caller
observing a bucket that is holding a blocking reservation sees a negative
count.

**Here, every read floors at zero.** A token count is a count; the API has no
reading below zero, and a caller that renders it into a `RateLimit-Remaining`
header or a dashboard has nothing sensible to do with `-5`.

The port had already deviated, but *inconsistently*, which is what forced the
decision: two of the three reads clamp with `max0` (`core/ops.go:40` and
`core/ops.go:55`, the probe reject branches), upstream clamps none of them, and
`core/ops.go:77` is the only read still faithful to upstream. Ruling one way or
the other was unavoidable; ruling toward the clamp makes one read agree with
the two beside it rather than making three reads agree with Java.

**The internal ledger stays signed.** `currentSize` still goes negative in
`reserve` (`core/ops.go:90`) — that is the mechanism that holds a blocking
waiter's place in line and it survives across nodes through the codec. Only its
*visibility* changes. Replacing the signed ledger with a never-negative
encoding (a GCRA-style "next permitted time" instead of a token count) was
**rejected**: it is a codec format change, and the one format break this
project has budgeted is the multi-bandwidth + config-fingerprint change planned
for v0.2. It is also not what upstream does — bucket4j has no GCRA anywhere,
only token counts, and the experimental IEEE754 state variant that once existed
was deleted.

## 2. A cancelled blocking reservation is refunded (AC-54)

**Upstream never refunds, and says so on purpose.** `BlockingBucket.java:47-51`
documents it in the method contract — if the thread is interrupted while
blocked, "tokens will be not returned back to bucket" — and offers
`Bucket#addTokens(long)` as the caller's escape hatch. The implementation
matches: `AbstractBucket.java:198-204` is a bare catch-and-rethrow with no
`finally` block. `BlockingConsumeSpecification.groovy:116-128` pins the
behaviour with a test, so it is not an oversight anyone drifted into.

**Here, the cancel branch releases the reservation** through the same executor
that took it, best-effort and reported. `core/ops.go:90` deducts before the
sleep and the write is persisted before any waiting starts, so on cancellation
the tokens are charged to a request nobody was served — every other caller is
rate-limited for traffic that did not happen. Under load, where cancellation is
normal rather than exceptional, that is a systematic under-count of capacity.

The Go context model is the reason the upstream answer does not transfer.
Upstream's escape hatch assumes an `InterruptedException` reaching application
code that knows how many tokens it asked for. In Go the cancellation is
routinely delivered by a middleware or an HTTP server the caller never wrote,
and `addTokens` is not reachable from there.

**Rejected: following upstream and documenting the leak.** That is what
`core/blocking.go:22-23` already did — "the reservation stands and is repaid by
refill" — and "repaid by refill" is true and beside the point. The bucket
recovers; the accounting was still wrong for the whole recovery window.

**Rejected: treating a failed release as a store failure.** It returns the bare
context error plus a dedicated sentinel, never `ErrStore`, because ADR 0015
already ruled that tagging a departed client's cancellation as a store failure
routes it into the caller's fail-open branch.

**Known ceiling, not solved:** a store write that commits on the wire but loses
its reply still over-counts. `core` cannot distinguish "did not apply" from
"applied, reply lost", and neither can any backend; closing it needs idempotency
keys in the stored state, i.e. the same v0.2 codec break named above.

## Why an ADR and not a comment

Both of these are one-line changes with a plausible-looking upstream source
arguing the opposite, and the upstream source is more discoverable than the
reasoning. Without this file the failure mode is concrete: a contributor reads
`BlockingBucket.java:47-51`, sees an explicit deliberate decision, and reverts
the refund as a port fidelity fix. Recording the trade-off is the only thing
that stops that.

One thing this ADR does **not** claim: that upstream's reservation design was
examined and found wanting. It was examined. The only stated rationale for
reservations in bucket4j is four words — "reserved in fair manner"
(`BlockingBucket.java:44`) — with no fairness, FIFO or thundering-herd
discussion anywhere in the asciidoc documentation or the commit history. The
mechanism is kept as ported; only its two observable edges move.
