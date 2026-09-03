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

## Ratified — the owner ruled for the bare context error

Decided. Two arguments settled it, neither of which is in the reasoning above.

**It was never about losing the sentinel.** `storeErr`
(`core/command.go:17-21`) wraps with a double `%w`:
`fmt.Errorf("%w: %w", ErrStore, err)`. Under the rejected option *both*
`errors.Is(err, ErrStore)` and `errors.Is(err, context.Canceled)` hold — no
information is destroyed either way. The only question was whether a
cancellation is additionally **classified** as a store failure, and the answer
is that it is not: a departed client and a dead backend call for different
handling, and one label for both makes the distinction a matter of which
`errors.Is` the caller happens to write first.

**Stdlib precedent, in this exact architecture.** `database/sql` fronts
pluggable drivers behind one interface — the same shape as `core` over
`CASStore`/`LockStore` — and its pre-flight context checks return a bare
`ctx.Err()`, never tagged as a driver or connection error:
`$(go env GOROOT)/src/database/sql/sql.go:1327` (`(*DB).conn`), `:1396`
(the connection-request wait), `:2249` (`(*Tx).grabConn`) and `:2297`
(`(*Tx).awaitDone`). Verified against the Go **1.25.6** source tree available
here (`$GOROOT/VERSION`), not the 1.23 the repo pins; the pattern long predates
both, but that is the tree that was actually read.

## Consequences to apply elsewhere

- **PR #12's guards become `return ctx.Err()`**, and its `core/bucket.go` doc
  edit — which argues the caller catches both under one
  `errors.Is(err, ErrStore)` — is dropped. To be applied in that PR by whoever
  owns it; this branch does not edit it.
- **AC-51 clause 1 needs the carve-out** described above. Same: applied in
  PR #11, not here.
- **Behaviour change for callers.** Code that checks only
  `errors.Is(err, ErrStore)` and fails open now falls through on cancellation
  and returns whatever its default branch does. That is the intended outcome —
  a request whose client has gone should not be served — but it is a change,
  not a no-op, and a caller that genuinely wants the old grouping must now
  write `errors.Is(err, ErrStore) || errors.Is(err, context.Canceled)`.
