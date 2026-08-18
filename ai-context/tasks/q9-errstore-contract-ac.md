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
