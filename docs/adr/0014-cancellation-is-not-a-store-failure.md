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

The `ErrStore` contract now has four clauses, tracked as **AC-51**: a store
failure is distinguishable from misuse and from refusal; the backend's own
error stays reachable through `errors.Is`/`errors.As`; a corrupt-state decode
failure is not a store failure (`core/command.go:100-102`); a cancellation is
not a store failure. `ErrCASRetriesExhausted` stays inside `ErrStore` — the
store is healthy but the operation did not complete through it, which is the
same decision for the caller.
