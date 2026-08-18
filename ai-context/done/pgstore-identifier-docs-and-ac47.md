# pgstore — document the identifier contract, give it AC-47

**Origin:** Q5 ruling, `docs/adr/0008-pgstore-identifiers-are-single-quoted-names.md`.
Doc + test-naming only — **no behavior change, no SQL change**. `quote()` stays
`pgx.Identifier{name}.Sanitize()`; the `strings.Split` variant was rejected.

**Goal:**

1. **AC-47.** SQL identifier safety had no acceptance criterion — that gap is
   now closed by assigning it one. Rename
   `TestHostileIdentifiersAreEscapedNotExecuted`
   (`pgstore/pgstore_test.go`) to `TestAC47_HostileIdentifiersAreEscapedNotExecuted`
   and drop the stale trailing line of its doc comment
   ("No acceptance criterion covers SQL identifier safety (see open question Q5)").
   AC-47 = *every configured identifier is escaped, never executed, and every
   interpolating statement agrees on the escaping — including the derived index
   name and the `information_schema` guard.* The existing assertions already
   cover it; nothing new to assert.

2. **Godoc on `WithTable`** (`pgstore/pgstore.go`): say the value is a single
   identifier, quoted verbatim, and therefore case-sensitive and not
   schema-qualifiable — for a non-default schema, set `search_path` on the
   connection and pass a bare name. One or two lines, not an essay.

3. **`docs/client/postgres.md`**: the case-sensitivity note (around the options
   table) already exists — extend it with the schema case, e.g. that
   `WithTable("myschema.bucket")` names a table *called* `myschema.bucket`
   rather than `bucket` in `myschema`, and point at `search_path`. This is the
   silent-surprise half of the ruling, so it must be written down where a user
   reads before configuring.

**Independence:** Self-contained in `pgstore` + `docs/client`. No change to
`core`, `redisstore`, or `examples`.

**Acceptance:** `/verify` green; `pgstore` suite passes against a real Postgres
container per `docs/adr/0006-testcontainers-for-backend-tck.md`, with
`TestAC47_...` present and passing under its new name.
