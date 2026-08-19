# PLANS.md — Distributed Token-Bucket Rate Limiter (Go)

> **Architect-owned.** Only the architect writes here. Living progress tracker:
> phases, status, and outcomes (decisions live in `docs/adr/`). Dev/investigator
> read it; they raise changes via a task file in `ai-context/tasks/`.

## Goal
Port bucket4j's token-bucket (shared-state, not queue) to Go. `core` module owns
the algorithm + API; `redisstore` and `pgstore` modules are pluggable backends.
**Primary design goal: swap provider without changing the rate-limiting logic.**

## Decisions
Ratified decisions live in `docs/adr/` now, not here — see
`docs/adr/0007-tasks-done-workflow.md` for why. Quick index of what's there:
single-bandwidth v1 (0001), core stays protocol-agnostic (0002), Postgres
SELECT-FOR-UPDATE over advisory locks (0003), TEXT bucket keys, no mapper
(0004), LocalBucket mutex over lock-free CAS (0005), testcontainers for
backend TCK (0006), pgstore identifiers are one quoted name (0008), pgstore
creates no row for a read-only miss (0010). Smaller settled facts that don't clear the ADR bar (module
path, Go toolchain installed by the user) aren't tracked anywhere formal —
they're just true of the repo.

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
Every acceptance criterion for the phase has a passing test (criteria were
tracked in `docs/internal/project/ACCEPTANCE_CRITERIA.md`, which no longer
exists in this workspace — the AC ids themselves survive as `TestAC<id>_...`
test names in the code; re-deriving a canonical criteria doc is out of scope
for this restructuring and, if wanted, belongs in its own task). `go build
./...` + `go vet ./...` clean. Backends pass the TCK
against a real container.

## Failure-handling loop
On any verification failure → Architect opens (or points at) a task in
`ai-context/tasks/` → Investigator appends findings to it (symptom, failing AC
id, root cause, defect-vs-spec-gap) → triggers Dev (code fix) or Architect
(spec/design fix) → loop until AC pass → **tester** agent independently reruns
`/verify` + a small end-to-end scenario and moves the file to
`ai-context/done/`. Architect records the outcome here.

## Open work (tracked as task files in ai-context/tasks/)
`docs/internal/project/QUESTIONS_TO_ARCHITECT.md` no longer exists (it was
gitignored and never present in this workspace); the four still-open items it
tracked are now task files, carrying forward what `PLANS.md` had recorded
about each:
- `ai-context/tasks/q5-pgstore-identifier-behavior-change.md` — **ruled**
  (ADR 0008): both behavior changes accepted as-is; `strings.Split` rejected.
  Follow-up doc/AC task done: `ai-context/done/pgstore-identifier-docs-and-ac47.md`.
- `ai-context/tasks/q6-pgstore-read-only-row-leak.md` — **ruled** (ADR 0010):
  the insert moves inside the locking transaction and runs only when the
  callback writes; both original candidates rejected. Implementation task:
  `ai-context/tasks/pgstore-no-row-for-read-only-miss-ac48.md` (AC-48 =
  storage footprint tracks writes, not reads).
- `ai-context/tasks/q8-lockstore-do-error-contract.md` — ratify what
  `LockStore.Do` may do to the callback's error (doc-only vs. TCK-enforced).
- `ai-context/tasks/q9-errstore-contract-ac.md` — assign an AC id to the
  `ErrStore` contract the 8 tests in `core/storeerr_test.go` currently test
  without one; blocked on Q8's ruling landing first.

Q1–Q4 and Q7 were already answered/retired before this session and are
folded into `docs/adr/` (0001-0006) rather than repeated here.

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
- _restructure_: `docs/internal/` (gitignored, absent from this workspace) retired.
  Ratified decisions migrated to `docs/adr/` (0001-0007). Replaced
  `problem_reports/` + per-question tracking with a single `ai-context/tasks/` →
  `ai-context/done/` lifecycle for both dev and investigation work, gated on a
  new **tester** role (`.claude/agents/tester.md`, model haiku) that
  independently reruns `/verify` plus a small end-to-end scenario before moving
  a task to `done/` — not self-attested by whoever did the work. Added `model:`
  to all four agent files (architect: opus, dev: sonnet, investigator: sonnet,
  tester: haiku). Q5/Q6/Q8/Q9 carried forward as task files, unresolved (ruling
  deferred to next session, per explicit decision to restructure first). Seeded
  three new-backend task files (MongoDB, etcd, Hazelcast) as an unordered,
  independent backlog; Memcached considered and dropped from scope for now.
- _q5_: pgstore identifier behavior ruled (ADR 0008) — case-sensitive names
  accepted, dotted schema-qualified names stay broken (`strings.Split` rejected:
  breaks `CREATE INDEX` naming and silently disables the `information_schema`
  BIGINT guard; `search_path` is the workaround). No SQL change; follow-up task
  covers the godoc/client-doc note and assigns AC-47 to identifier safety.
- _q6_: pgstore read-only row leak ruled (ADR 0010) — placeholder insert stops
  being committed up front; it moves inside the locking transaction and runs
  only when the callback writes, so a read of an absent key persists nothing
  (parity with Redis/memstore). Short-`expires_at` and existence-probe
  candidates rejected. Follow-up task carries the code change, AC-48, and the
  `state IS NULL` cleanup note for tables written by the old build.
