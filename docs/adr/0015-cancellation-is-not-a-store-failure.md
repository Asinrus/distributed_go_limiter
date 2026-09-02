# Cancellation is not a store failure

A distributed bucket operation aborted by a cancelled or expired context
returns the bare `context.Canceled` / `context.DeadlineExceeded`, **not**
`core.ErrStore`. `errors.Is(err, core.ErrStore)` is false for it.

`ErrStore` exists so a caller can pick fail-open vs fail-closed when the
backend is unreachable (ADR 0002). A client that hung up is not an outage:
failing open there admits a request nobody is waiting for, and — worse — a mass
disconnect (exactly what happens under the load a limiter is for) becomes
indistinguishable from Redis being down, so "is my store healthy?" stops being
answerable from the error. Callers already reach for
`errors.Is(err, context.Canceled)` because that is what `database/sql` and the
drivers return; matching the stdlib shape costs nothing.

The two producers of this error must therefore agree. `storeErr`
(`core/command.go:17`) wraps everything crossing the store boundary, so a
driver that notices the cancellation first produces `ErrStore` today; an
executor guard that notices first would produce a bare context error. Same
cause, two shapes, decided by a race. `storeErr` is the single choke point: if
the context is done it returns the context's error unwrapped and drops the
underlying one. Dropping it is deliberate — the discarded error is almost
always the driver's own `context.Canceled`, and preserving it would put a
second, differently-shaped error on the same path for no caller who can act
on it.

Rejected: **`ErrStore`-wrapped, so both `errors.Is` checks hold.** It is
strictly more information, but only if the caller tests `context.Canceled`
first. The documented check is `ErrStore`, so the common ordering routes every
cancellation into the outage branch — the failure this decision exists to
prevent. A caller who genuinely wants cancellation treated as a store failure
can still write that; it should have to say so.

Rejected: **guard in each store rather than in the executor.** Then honouring
the context is a property of how diligent each driver is, and the window
between a successful read and the swap stays unhandled in every backend.

## Consequences

`core/bucket.go:19-24` drops "cancellation" from `ErrStore`'s enumerated
causes and names the context errors instead. This narrows ADR 0002's "store
failures surface only as `core.ErrStore`" — that sentence is about transport
and degradation policy, and it stands for everything except cancellation.

AC-51, the `ErrStore` criterion ratified in
`ai-context/tasks/q9-errstore-contract-ac.md`, states as clause 1 that *every*
failure originating at the store boundary surfaces as `ErrStore`. This decision
is a **carve-out from that clause**, not a new criterion and not a new id: a
cancellation is the one store-boundary failure that does not. Everything else
in AC-51 stands, including `ErrCASRetriesExhausted` inside `ErrStore` — there
the store is healthy but the operation did not complete through it, which is
the same decision for the caller.

## Unresolved: open PR #12 implements the opposite

**This ADR is not safe to merge on its own.** Open PR #12
(`asinrus/ctx-cancellation`) already implements AC-50 with the rejected option:
its executor guards return `storeErr(ctx.Err())`, and it edits `core/bucket.go`
to argue that a caller catches cancellation and outage together under one
`errors.Is(err, ErrStore)`. That is internally consistent — both the guard and
the driver path wrap, so it has no two-shapes problem — and it agrees with
AC-51 clause 1 as PR #11 words it. It is simply the opposite trade-off: it
optimises for one check at the call site, this ADR optimises for a caller being
able to tell a disconnected client from a dead backend.

Both cannot merge. The choice is the repo owner's, and it is a one-line
decision with a three-file consequence: pick a shape, then either PR #12's
`core/command.go` guards or this ADR plus the `storeErr` choke point it
requires is rewritten, and AC-51 clause 1 either grows the carve-out above or
does not. Nothing else in `ai-context/tasks/core-ctx-cancellation.md` depends
on the answer, and nothing in
`ai-context/tasks/tck-vendor-ctx-conformance.md` (AC-52) does either — its
assertions are at the store boundary, below where `ErrStore` is applied.
