# PLANS.md — Distributed Token-Bucket Rate Limiter (Go)

> **Architect-owned.** Only the architect writes here. Living progress tracker:
> phases, status, decisions, and outcomes. Dev/investigator read it; they raise
> changes via `docs/internal/project/QUESTIONS_TO_ARCHITECT.md` or `problem_reports/`.

## Goal
Port bucket4j's token-bucket (shared-state, not queue) to Go. `core` module owns
the algorithm + API; `redis` and `postgres` modules are pluggable backends.
**Primary design goal: swap provider without changing the rate-limiting logic.**

## Confirmed decisions
- **v1 = single bandwidth** per bucket. Multi-bandwidth deferred.
- Scope: lean + blocking. Deferred: multi-bandwidth, async, verbose, optimization
  layers, config replacement/versioning, pluggable math, cross-language wire compat.
- Module path: `github.com/asinrus/distributed_go_limiter/{core,redis,postgres}`.
- Tests: testcontainers-go (assumes Docker-compatible runtime at test time).
- Go toolchain installed by the user.
- Postgres v1: SELECT FOR UPDATE strategy only (advisory lock deferred).

## Phases & status
| # | Phase | Status | Notes |
|---|-------|--------|-------|
| 0 | Investigation (docs/internal/bucket4j) | ✅ done | 3 agent reports + Infinispan article |
| 1 | Architect docs (docs/internal/project) | ✅ done | ARCHITECTURE (+mermaid, swap examples), EXT_*, ACCEPTANCE_CRITERIA, QUESTIONS |
| 2 | core: state/bandwidth/config/probe (TDD) | ✅ done | AC-11..19 tests written |
| 3 | core: LocalBucket + BlockingBucket (TDD) | ✅ done | AC-21..23, AC-31/32 tests written |
| 4 | core: Store/ProxyManager/command/codec/memstore + TCK | ✅ done | AC-41..46 via in-memory; codec + AC-45 TTL |
| 5 | redis backend + testcontainers TCK | ✅ verified | Lua CAS + SetNX; TCK + AC-45 green on real Redis |
| 6 | postgres backend + testcontainers TCK | ✅ verified | SELECT FOR UPDATE; TCK + AC-45/RemoveExpired green on real Postgres |

## Verification — DONE (Go 1.26.5, podman/testcontainers)
All green:
- `core`: `go vet` clean; `go test -race ./...` PASS (AC-11..46 incl. concurrency, blocking, in-memory TCK).
- `redis`: TCK AC-41..46 + AC-45 expiration PASS on `redis:7-alpine`.
- `postgres`: TCK AC-41..46 + AC-45 + RemoveExpired PASS on `postgres:16-alpine`.

One defect found & fixed during verification (impl-level, no problem_report needed):
`testcontainers.TerminateContainer` doesn't exist in v0.33.0 → switched both backend
tests to `ctr.Terminate(ctx)`.

### How to run (this machine)
```
export PATH="$HOME/.local/go/bin:$PATH" GOTOOLCHAIN=local
export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/podman/podman.sock" TESTCONTAINERS_RYUK_DISABLED=true
# start once: podman system service --time=0 "unix://$XDG_RUNTIME_DIR/podman/podman.sock" &
cd core && go test -race ./...      # no container
cd ../redis && go test ./...        # testcontainers Redis
cd ../postgres && go test ./...     # testcontainers Postgres
```
Note: repo root is go.work-only (not a module), so build per-module (`cd core && go build ./...`),
not `go build ./...` from root. podman needs `~/.config/containers/registries.conf`
with `unqualified-search-registries = ["docker.io"]` for short image names.

Legend: ✅ done · 🟡 in progress · ⬜ pending · ❌ blocked.

## Definition of done (per phase)
Every acceptance criterion for the phase (see `docs/internal/project/ACCEPTANCE_CRITERIA.md`)
has a passing test. `go build ./...` + `go vet ./...` clean. Backends pass the TCK
against a real container.

## Failure-handling loop
On any verification failure → investigator writes
`docs/internal/project/problem_reports/<phase>-<n>.md` (symptom, failing AC id, root cause,
defect-vs-spec-gap) → triggers Dev (code fix) or Architect (spec/design fix) →
loop until AC pass. Architect records outcome here.

## Open questions (tracked in QUESTIONS_TO_ARCHITECT.md)
- Q1–Q4 answered — TEXT keys (no mapper, rejected the BIGINT/VARCHAR split),
  TTL shipped on both backends, in-memory ProxyManager shipped and exported,
  LocalBucket stays mutex-based (measured against atomic.Pointer CAS).
- Q5 (open): Postgres table/column names reach SQL via `fmt.Sprintf`, unescaped.
  Low priority under the current threat model (app-supplied, not end-user input).
- Q6 (open): Postgres leaks a permanent placeholder row for any key that is only
  ever read (`AvailableTokens`/`EstimateAbilityToConsume`), never written.

## Changelog
- _init_: phases defined; investigation complete; architect docs started.
- _impl_: phases 2–6 implemented TDD; all core + backend code written.
- _verify_: Go 1.26.5 installed userspace; full suite green on all 3 backends via
  podman/testcontainers. Fixed testcontainers `TerminateContainer` API mismatch.
