# Q5 — ratify pgstore identifier-sanitization behavior change

**Origin:** `pgstore/pgstore.go:67` now routes every configured identifier
(table/schema name) through `pgx.Identifier{}.Sanitize()` to fix an injection
found by `TestHostileIdentifiersAreEscapedNotExecuted`
(`pgstore/pgstore_test.go:85`). That fix is already in the code. Two behavior
changes it introduces are unratified:

1. `WithTable("MyTable")` is now case-sensitive (it wasn't before).
2. `WithTable("myschema.bucket")` now breaks (dotted schema-qualified names).
   A one-line fix is available: `pgx.Identifier(strings.Split(name, "."))`.

**Goal:** Architect rules on (1) accept as-is, and (2) accept the break, or
apply the one-line `strings.Split` fix to restore dotted-name support.

**Acceptance:** Ruling recorded (ADR if it clears the bar — likely yes, it's
a security fix with a real behavior trade-off). If the split fix is chosen,
Dev implements it and existing pgstore tests plus a new case for dotted
names pass.

## Ruling — both accepted as-is, no SQL change

Recorded in `docs/adr/0008-pgstore-identifiers-are-single-quoted-names.md`.

1. **Case-sensitive names: accepted.** Un-quoting to restore folding
   reintroduces the injection. Lower-casing before quoting silently rewrites a
   caller who genuinely owns a mixed-case table, and `EnsureSchema`'s legacy-
   BIGINT guard depends on the catalog holding the configured string verbatim.
   Already documented in `docs/client/postgres.md`.
2. **Dotted schema-qualified names: break accepted, `strings.Split` rejected.**
   It is not a one-line fix. `CREATE INDEX` forbids a schema-qualified index
   name, so the derived `<table>_expires_at_idx` identifier in `SchemaSQL` needs
   different splitting from the table's; and `EnsureSchema`'s
   `information_schema` lookup matches on `table_name` alone, so a split table
   name would make the BIGINT guard stop firing *silently*. Three call sites and
   a semantic hole to duplicate what `search_path` on the DSN/pool/role already
   does. Revisit only if one pool must address bucket tables in two schemas at
   once.

**Follow-up (doc + test naming only, done):**
`ai-context/done/pgstore-identifier-docs-and-ac47.md` — godoc + client-doc note
on the schema case, and AC-47 assigned to the identifier-safety test that
previously had no AC id. Implemented by Dev, verified by the tester agent
(`pgstore` `/verify` + `examples` build smoke check), moved to `done/`.

This file needed no code change and no e2e run of its own; the follow-up task
carried the `/verify` gate.
