# The tester agent verifies behavior, not code

(Numbered 0011, not 0009: `0009`/`0010` are reserved by two concurrently
developed branches — `asinrus/q8-lockstore-error-contract` and
`asinrus/q6-pgstore-row-leak` — not yet merged when this was written. Not a
gap; they land in order.)

The tester's "end-to-end" gate (`.claude/agents/tester.md`) initially ran `go
test`/`go vet` and read source/database state directly — that's a second unit-
and-integration pass, not an independent behavioral check, and it let a
reviewer-caught bug (Q6's row-leak) get "e2e-verified" by a test that never
ran the compiled application. The tester now treats the system as a black
box: it builds and runs the real `examples/redis` or `examples/postgres`
binary against a real backend container, drives it with real HTTP traffic
via [k6](https://k6.io) (`e2e/k6/ratelimit_test.js`, run from the
official `grafana/k6` image — no local k6 install required), and asserts on
observed status codes only. It has no `Read`/`Grep` tools; it cannot look at
source or query the database directly, only issue requests and check
responses.

k6 over Gatling: single container image, no JVM dependency in a Go-only
repo, and the check here — count accepted vs. rejected responses against a
known capacity — doesn't need Gatling's heavier reporting.

`/verify` (compile + `go test`) is unaffected by this — it remains a
legitimate, separate correctness gate. This ADR only redefines what
"end-to-end" means for the tester's second gate.
