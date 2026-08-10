# PLANS.md — Distributed Token-Bucket Rate Limiter (Go)

> **Architect-owned.** Only the architect writes here. Living progress tracker:
> phases, status, decisions, and outcomes. Dev/investigator read it; they raise
> changes via `docs/internal/project/QUESTIONS_TO_ARCHITECT.md` or `problem_reports/`.

## Goal
Port bucket4j's token-bucket (shared-state, not queue) to Go. `core` module owns
the algorithm + API; `redisstore` and `pgstore` modules are pluggable backends.
**Primary design goal: swap provider without changing the rate-limiting logic.**

## Confirmed decisions
- **v1 = single bandwidth** per bucket. Multi-bandwidth deferred.
- Scope: lean + blocking. Deferred: multi-bandwidth, async, verbose, optimization
  layers, config replacement/versioning, pluggable math, cross-language wire compat.
- Module path: `github.com/asinrus/distributed_go_limiter/{core,redisstore,pgstore,examples}`.
- Tests: testcontainers-go (assumes Docker-compatible runtime at test time).
- Go toolchain installed by the user.
- Postgres v1: SELECT FOR UPDATE strategy only (advisory lock deferred).
- **`core` is protocol-agnostic**: no HTTP, gRPC, or any transport inside the library —
  an adapter belongs in the caller. Store failures wrap `core.ErrStore` so the caller
  owns the fail-open vs fail-closed decision. core never picks a status code.

## Phases & status
| # | Phase | Status | Notes |
|---|-------|--------|-------|
| 0 | Investigation (docs/internal/bucket4j) | ✅ done | 3 agent reports + Infinispan article |
| 1 | Architect docs (docs/internal/project) | ✅ done | ARCHITECTURE (+mermaid, swap examples), EXT_*, ACCEPTANCE_CRITERIA, QUESTIONS |
| 2 | core: state/bandwidth/config/probe (TDD) | ✅ done | AC-11..19 tests written |
| 3 | core: LocalBucket + BlockingBucket (TDD) | ✅ done | AC-21..23, AC-31/32 tests written |
| 4 | core: Store/ProxyManager/command/codec/memstore + TCK | ✅ done | AC-41..44 via in-memory; codec + AC-45 TTL |
| 5 | redis backend + testcontainers TCK | ✅ verified | Lua CAS + SetNX; TCK + AC-45 green on real Redis |
| 6 | postgres backend + testcontainers TCK | ✅ verified | SELECT FOR UPDATE; TCK + AC-45/RemoveExpired green on real Postgres |

## Verification — DONE (Go 1.26.5, podman/testcontainers)
All green:
- `core`: `go vet` clean; `go test -race ./...` PASS (AC-11..46 incl. concurrency, blocking, in-memory TCK).
- `redisstore`: TCK AC-41..44 + AC-45 expiration PASS on `redis:7-alpine`.
- `pgstore`: TCK AC-41..44 + AC-45 + RemoveExpired PASS on `postgres:16-alpine`.

One defect found & fixed during verification (impl-level, no problem_report needed):
`testcontainers.TerminateContainer` doesn't exist in v0.33.0 → switched both backend
tests to `ctr.Terminate(ctx)`.

### How to run (this machine)
```
export PATH="$HOME/.local/go/bin:$PATH" GOTOOLCHAIN=local
export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/podman/podman.sock" TESTCONTAINERS_RYUK_DISABLED=true
# start once: podman system service --time=0 "unix://$XDG_RUNTIME_DIR/podman/podman.sock" &
cd core && go test -race ./...      # no container
cd ../redisstore && go test ./...    # testcontainers Redis
cd ../pgstore && go test ./...       # testcontainers Postgres
```
Note: repo root is go.work-only (not a module), so build per-module (`cd core && go build ./...`),
not `go build ./...` from root. podman needs `~/.config/containers/registries.conf`
with `unqualified-search-registries = ["docker.io"]` for short image names.

Legend: ✅ done · 🟡 in progress · ⬜ pending · ❌ blocked · 🗑 removed.

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
- Q5 (partly closed): Postgres table/column names reached SQL via `fmt.Sprintf`,
  unescaped. **Fixed in code** — `pgstore/pgstore.go:67` routes every configured
  identifier through `pgx.Identifier{}.Sanitize()`. The earlier "low priority under
  the current threat model" assessment was **wrong**: `TestHostileIdentifiersAreEscapedNotExecuted`
  (`pgstore/pgstore_test.go:85`) shows a pre-fix injection reached the Postgres parser.
  **Ruling pending** on two behaviour changes the fix introduces: (a) `WithTable("MyTable")`
  is now case-sensitive; (b) `WithTable("myschema.bucket")` now breaks — one-line fix
  available (`pgx.Identifier(strings.Split(name, "."))`). Accept, or restore either?
- Q6 (open): Postgres leaks a permanent placeholder row for any key that is only
  ever read (`AvailableTokens`/`EstimateAbilityToConsume`), never written.
  Report: `docs/internal/project/problem_reports/q6-1.md` — routed as a **spec gap**
  (no AC covers storage footprint): `SELECT … FOR UPDATE` needs a row to lock, so
  pgstore commits it with `expires_at` NULL before the read-only callback runs and
  the sweep can never reclaim it; Redis/memstore create nothing on the same path,
  so the two backends differ observably. Two candidate fixes in §3 — **decision pending**.
- Q7 (retired — **closed by removal**): asked which AC block should cover an HTTP
  middleware in `core` and whether its fail-closed default was deliberate. Ruling:
  core is protocol-agnostic, so the middleware left the library and the degradation
  choice moved to the caller via `errors.Is(err, core.ErrStore)`. The proposed
  AC-51..57 block is dropped; the one criterion that survives it is tracked as Q9.
- Q8 (open, from Dev): `LockStore.Do` (`core/store.go:34-35`) does not specify what
  a store must do with the callback's error. The decode-vs-outage guard in
  `lockRunner.execute` (`core/command.go:100`) matches with `errors.Is`, so a store
  that *wraps* fn's error is fine, but a store that *replaces* it would get its own
  decode failures mis-tagged `ErrStore`. Proposed contract wording: *"Do must return
  fn's error unmodified or wrapped, never replaced."* **Not ratified** — needs a
  ruling on whether this becomes doc-only or also an AC the TCK enforces.
- Q9 (open): the 8 tests in `core/storeerr_test.go` map to no AC id and are named
  descriptively, so nothing in the spec pins the `ErrStore` contract. Criterion
  needed: *a store-layer failure is distinguishable from misuse and from refusal;
  the backend error remains unwrappable; a corrupt-state decode failure is NOT
  reported as a store failure.* The last clause is the non-obvious half (see the
  guard at `core/command.go:85-102`). **AC id not yet assigned** — numbering to be
  ratified with Q8, then the tests renamed `TestAC…`.

## Changelog
- _init_: phases defined; investigation complete; architect docs started.
- _impl_: phases 2–6 implemented TDD; all core + backend code written.
- _verify_: Go 1.26.5 installed userspace; full suite green on all 3 backends via
  podman/testcontainers. Fixed testcontainers `TerminateContainer` API mismatch.
- _reconcile_: module names corrected to `redisstore`/`pgstore` (f3eeeac); Q6 linked
  to problem report q6-1.
- _agents_: roles now exist as subagents (`.claude/agents/`, `/verify`) and are
  indexed from `ai-context/README.md`; Q5 corrected (identifiers fixed in code,
  behaviour-change ruling pending).
- _protocol-agnostic core_: an HTTP middleware briefly added to `core` was deleted
  and Q7 retired by removal; store failures now surface as `core.ErrStore` (caller
  owns the degradation policy); Q8 (`LockStore.Do` error contract) and Q9 (missing
  `ErrStore` AC) opened.
- _review_: PR #1 review addressed — comments culled repo-wide, client docs moved to
  `docs/client/`, `pgstore.SchemaSQL` added so users can apply the schema with their
  own migration tool, `/verify` made module-agnostic.
