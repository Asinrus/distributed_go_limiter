# Bump grpc to close GO-2026-6061, raising two modules' `go` directive

**The ruling.** The owner resolved the conflict this task originally
documented: `google.golang.org/grpc` moves to v1.82.1 to close GO-2026-6061.
v1.82.1 requires `go 1.25.0`, above the project-wide `go 1.23` pin
(`ai-context/tasks/go-version-1-23.md`, `docs/adr/0014-single-go-version.md`).
Per that task's own escape hatch — raise only the module that genuinely needs
a post-1.23 floor, and record which feature forced it — only the modules that
actually carry grpc were raised. Everything else stays at `go 1.23`.

**What was verified, not just carried over from the prior investigation.**
`go mod graph` shows `google.golang.org/grpc` in every module's graph,
including `redisstore`, `pgstore`, and `mongostore` — all three pull it
transitively through `testcontainers-go`, a test-only dependency. `govulncheck`
(which does reachability analysis, not just graph presence) confirms GO-2026-6061
is not reachable from any code path in those three modules, before or after
the bump — only `etcdstore` (via `go.etcd.io/etcd/client/v3`) and `examples`
(via the etcd client and testcontainers-go) actually call into the vulnerable
grpc transport code. `core` has no grpc in its graph at all.

**What changed:**
- `etcdstore/go.mod`: `google.golang.org/grpc` → v1.82.1; `go` directive →
  `1.25.0` (raised by `go mod tidy` to satisfy grpc's own `go.mod`, not
  hand-picked). `go.sum` updated to match.
- `examples/go.mod`: same grpc bump, same `go` directive raise, same reason
  (it pulls grpc via the etcd client and testcontainers-go). `go.sum` updated.
- `go.work`: `go` directive raised to `1.25.0` — the minimum that satisfies
  its highest member (`etcdstore`/`examples`). No `toolchain` directive was
  added anywhere, consistent with the standing constraint.
- `core/go.mod`, `redisstore/go.mod`, `pgstore/go.mod`, `mongostore/go.mod`:
  unchanged, still `go 1.23`.
- `docs/adr/0014-single-go-version.md`: amended with a short note recording
  which two modules were raised, to what, and that a transitive grpc security
  floor forced it, not a language feature.

## Test scenarios

**Black-box (tester, Gate 2):** none beyond "etcd still rate-limits
correctly" — this is a dependency version bump with no behavior change, so
there is no new black-box surface to exercise.

**Unit / integration (Dev):**
- Per module (`core`, `redisstore`, `pgstore`, `mongostore`, `etcdstore`):
  `go vet ./...` and `go test ./...` (`core` with `-race`) pass unchanged —
  the bump does not touch the algorithm or any store's behavior.
- `examples`: `go vet ./...` and `go build ./...` pass (no test files).
- `etcdstore`'s full suite, including the shared `core/tck` conformance
  suite run against a real etcd container (`AC42_ConsistencyUnderContention`
  among its subtests), passes after the bump — a dependency change that
  breaks conformance would not be done.
- `govulncheck ./...` in `etcdstore` and `examples`: GO-2026-6061 present
  before the bump, absent after, in both.
