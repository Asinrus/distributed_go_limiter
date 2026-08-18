# pgstore identifiers are one quoted name each: case-sensitive, not schema-qualifiable

`WithTable` / `WithIDColumn` / `WithStateColumn` reach SQL by interpolation —
identifiers cannot be bind parameters — so every one of them goes through
`pgx.Identifier{name}.Sanitize()`. That is not optional; without it a
configured name closes the quote and runs a second statement. Two consequences
were ratified rather than worked around.

**Case-sensitive names, accepted.** Quoting an identifier suppresses Postgres's
fold-to-lowercase, so `WithTable("MyTable")` now means `"MyTable"`, not
`mytable`. Restoring the old behaviour would mean lower-casing the name
ourselves before quoting, which silently rewrites a caller who genuinely owns a
mixed-case table and buys nothing but the appearance of the pre-fix behaviour.
The exactness is also load-bearing: `EnsureSchema`'s legacy-BIGINT guard looks
the column up in `information_schema` by the *raw* configured string, which
only matches because the DDL stores that string verbatim.

**Dotted schema-qualified names, rejected.** `WithTable("myschema.bucket")` now
names one table whose name contains a dot, not `bucket` in `myschema`. The
proposed `pgx.Identifier(strings.Split(name, "."))` is not the one-line fix it
looks like: `CREATE INDEX` forbids a schema-qualified index name (the index
always lands in its table's schema), so the derived
`<table>_expires_at_idx` identifier would have to be split apart differently
from the table one; and the `information_schema` guard would need a
`table_schema` predicate or it stops firing — silently, which is worse than the
loud break. Three call sites and a semantic hole, to duplicate something the
connection already does: set `search_path` on the DSN, pool, or role and pass a
bare table name. Revisit if a caller needs one pool to address bucket tables in
two schemas at once, which v1 has no evidence of.

The escaping is covered by AC-47 (`TestAC47_...` in `pgstore/pgstore_test.go`),
which asserts a hostile identifier is escaped and never executed across every
interpolating statement, including the derived index name and the
`information_schema` guard.
