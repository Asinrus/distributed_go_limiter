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
