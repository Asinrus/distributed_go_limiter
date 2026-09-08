# Q9 — assign an AC id to the ErrStore contract

**Origin:** The 8 tests in `core/storeerr_test.go` map to no AC id and are
named descriptively rather than `TestAC<id>_...`. Nothing in the spec
currently pins the `ErrStore` contract down. Needed criterion, roughly: *a
store-layer failure is distinguishable from misuse and from refusal; the
backend error remains unwrappable; a corrupt-state decode failure is NOT
reported as a store failure* — that last clause is the non-obvious half (see
the guard at `core/command.go:85-102`), and it's entangled with Q8's ruling
on what `LockStore.Do` may do to `fn`'s error.

**Goal:** Architect ratifies the criterion wording (blocked on Q8's ruling
first — do it in the same session as Q8, not before), assigns it a stable AC
id, and Dev renames the 8 tests in `core/storeerr_test.go` to
`TestAC<id>_...`.

**Acceptance:** AC id exists wherever this project's acceptance criteria are
now tracked; all 8 tests renamed and still passing.

## Ruling — AC-51, ErrStore contract ratified

Q8 is resolved (`docs/adr/0009-lockstore-do-propagates-fn-error.md`), so the
entanglement noted above is cleared. Criterion, backed one clause at a time by
the 8 tests in `core/storeerr_test.go`:

1. Every failure originating at the store boundary — CAS `GetState`,
   `CompareAndSwap`, `Remove` on either backend, `LockStore.Do` — surfaces as
   `errors.Is(err, core.ErrStore)`, **except a cancelled or expired context**.
   Per `docs/adr/0015-cancellation-is-not-a-store-failure.md`, that one case
   returns the bare `context.Canceled` / `context.DeadlineExceeded` from the
   executor's choke point and `errors.Is(err, core.ErrStore)` is false for it —
   including when the store also failed with a backend error of its own, which
   the choke point discards. A departed client is not an outage and must not
   route into the caller's fail-open branch.
   (`TestAC51_GetStateFailureIsStoreError`,
   `TestAC51_CompareAndSwapFailureIsStoreError`,
   `TestAC51_RemoveFailureIsStoreError`,
   `TestAC51_LockStoreDoFailureIsStoreError`)
2. CAS retries exhausted is a store-layer failure too, and is additionally
   distinguishable via `errors.Is(err, core.ErrCASRetriesExhausted)`, itself
   wrapping `ErrStore`. Clause 1's carve-out does not reach it: the store is
   healthy but the operation did not complete through it, which is the same
   decision for the caller. (`TestAC51_CASRetriesExhaustedIsStoreError`)
3. A decode failure on corrupt stored bytes is core's own callback error — it
   only travels back out *through* the store's return value — and must NOT be
   reported as `ErrStore`, on either backend. This is the non-obvious half,
   and only holds because ADR-0009 pins `LockStore.Do` to returning `fn`'s
   error unmodified or `%w`-wrapped; `command.go`'s
   `errors.Is(err, ownErr)` guard (core/command.go:99) depends on that chain
   staying intact. (`TestAC51_CorruptStateIsNotStoreError`)
4. Misuse (`ErrNonPositiveTokens`) and plain refusal (`ok == false`, no
   error) never satisfy `errors.Is(err, core.ErrStore)`.
   (`TestAC51_MisuseAndRefusalAreNotStoreErrors`)
5. Wrapping in `ErrStore` never hides the backend's own error: it stays
   reachable through the same returned error via both `errors.Is` (for a
   backend that exposes a sentinel) and `errors.As` (for a concrete backend
   error type). (`TestAC51_OriginalBackendErrorStaysReachable`)

Every clause above has a test backing it directly; none of the 8 tests was
left uncovered by the wording, and no clause was written that the tests don't
already prove.

**Scoping AC-51 to also restate `LockStore.Do`'s error-propagation
obligation, rejected.** That's `Do`'s own contract, already ratified in
ADR-0009 as a godoc clause on the interface, aimed at store implementers.
AC-51 is scoped to what's observable at the `Bucket`/`ErrStore` boundary from
a caller's side — restating the store-implementer obligation here would just
duplicate the ADR under a different id and risk the two drifting apart.

**No ADR of its own** (clause 1's carve-out is ADR-0015's, ratified separately).
Nothing here changes behavior or code — `storeerr_test.go`'s
assertions already match this wording exactly, and the mechanism (the
`ownErr` guard) is already documented as ADR-0009's consequence. This is an
id assignment over existing, already-agreed behavior, the same shape as
AC-47's assignment in `ai-context/done/pgstore-identifier-docs-and-ac47.md`.

**Clause 1's carve-out is not tested here.** `core/storeerr_test.go` keeps its
8 cases; `TestAC51_MisuseAndRefusalAreNotStoreErrors` does not grow a
cancellation case. That test covers failures that never reach the store at all
(misuse, plain refusal), whereas the carve-out is about a failure that *does*
cross the boundary — and AC-50 already owns the both-directions assertion
(`errors.Is(err, context.Canceled)` true, `errors.Is(err, core.ErrStore)`
false, on both runners) in `ai-context/tasks/core-ctx-cancellation.md`. Adding
it here would duplicate that against the same choke point under a second id.
Consequence: clause 1's exception is not executable until the executor guard
from ADR-0015 lands; AC-50's tests are its executable form.

**Next:** rename the 8 tests in `core/storeerr_test.go` to `TestAC51_...`,
carried in this same branch (see PR).
