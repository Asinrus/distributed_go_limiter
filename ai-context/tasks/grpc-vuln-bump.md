# etcdstore's grpc dependency carries GO-2026-6061, and its fix conflicts with the go 1.23 pin

**Where this came from.** PR #9 (`asinrus/github-actions-ci`) added a
per-module `govulncheck` step to CI. Its first-ever run found this — it is
not from the planning round, and the owner has ruled PR #9 blocks on it.

**The vulnerability.** `etcdstore` requires `google.golang.org/grpc v1.64.1`
(transitively, via `go.etcd.io/etcd/client/v3`, and again via
`testcontainers-go` in tests). GO-2026-6061 (xDS RBAC authorization engine
and HTTP/2 transport server) affects every version below **v1.82.1**, per the
OSV record (`https://vuln.go.dev/ID/GO-2026-6061.json`): a single affected
range, `introduced: 0`, `fixed: 1.82.1`. There is no earlier patched release
to fall back to.

`examples/go.mod` carries the same `google.golang.org/grpc v1.64.1` line
(pulled in the same way, via the etcd client and testcontainers) and is
affected identically. `mongostore/go.mod` has no `google.golang.org/grpc`
entry at all, direct or transitive — confirmed with
`grep -rn 'google.golang.org/grpc' */go.mod` and `go mod graph` per module —
so it is not affected. `redisstore`, `pgstore`, and `core` don't depend on
grpc either.

**The conflict.** `google.golang.org/grpc@v1.82.1`'s own `go.mod` declares
`go 1.25.0`. `ai-context/tasks/go-version-1-23.md` (PR #13,
`asinrus/go-1-23-pin`) is a separate, already-ruled owner decision pinning
every module to `go 1.23`; that ruling measured `etcdstore`'s floor at
`go1.22` using the *pre-bump* dependency set and is not this task's to
reopen.

Verified directly rather than inferred from the declared directive: with
`etcdstore/go.mod`'s `go` line temporarily set to `1.23` and
`google.golang.org/grpc` bumped to `v1.82.1`, `go mod tidy` (GOTOOLCHAIN=local,
go1.26.5 installed) did not error — it silently rewrote the `go` line from
`1.23` to `1.26.5` to satisfy grpc's `go >= 1.25.0` requirement. That
auto-rewrite is exactly what the parent task told this agent not to do
silently, so it was reverted; `etcdstore/go.mod` and `go.sum` are unchanged
from `main`.

**Net result:** no version of `google.golang.org/grpc` both fixes
GO-2026-6061 and keeps `etcdstore` (or `examples`) buildable at `go 1.23`.
This is a genuine conflict between two owner rulings — routed to the
Architect to resolve, not decided here. Candidates worth putting in front of
the owner: raise `etcdstore`'s (and `examples`') `go` directive above 1.23
as a named, recorded exception (the 1.23 task's own text allows a
per-module raise "if a module genuinely needs a post-1.23 feature... record
which feature forced it"); wait on an upstream grpc backport if one appears;
or accept the open advisory in `etcdstore` until one of the above lands.

**Scope of this task as filed:** investigation and documentation only. No
`go.mod`/`go.sum` change was committed to `etcdstore` or `examples` — see
above for why. No new code was written.

## Test scenarios

**Black-box (tester, Gate 2):** none. No behavior changed; this task is
report-only pending an Architect decision.

**Unit / integration (Dev):** none added, since no fix was applied. The
verification performed was diagnostic, not a regression check:
- `govulncheck ./...` in `etcdstore` against `main`'s unmodified `go.mod`
  confirms GO-2026-6061 is present (`google.golang.org/grpc@v1.64.1`, fixed
  in `v1.82.1`).
- A scratch `go mod tidy` with `go 1.23` + `grpc v1.82.1` in `etcdstore`
  (not committed) confirms the toolchain silently raises the `go` directive
  rather than building at 1.23, which is the evidence for the conflict above.

Once the Architect rules on a path forward, whichever module's `go.mod`
changes needs the full existing treatment: `go build`, `go vet`, `go test`
against a real etcd container, and a rerun of `govulncheck` to confirm the
advisory clears.
