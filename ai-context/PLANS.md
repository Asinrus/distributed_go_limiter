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
creates no row for a read-only miss (0010), tester is black-box only (0011),
TTL is not a correctness mechanism (0012), Architect scopes a task's test
coverage (0013), cancellation is not a store failure (0015, contested — see
below). Smaller settled facts that don't clear the ADR bar (module
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
| 7 | mongodb backend + testcontainers TCK | ✅ verified | conditional `ReplaceOne` CAS; TTL index reclaims only, expiry in predicate; TCK green + k6 sandbox `mongodb` |
| 8 | etcd backend + testcontainers TCK | ✅ verified | `Txn` on `clientv3.Value`; per-write leases as expiry, no setup step; TCK green + k6 sandbox `etcd` |

## Verification — DONE (Go 1.26.5, podman/testcontainers)
All green:
- `core`: `go vet` clean; `go test -race ./...` PASS (AC-11..46 incl. concurrency, blocking, in-memory TCK).
- `redisstore`: TCK AC-41..44 + AC-45 expiration PASS on `redis:7-alpine`.
- `pgstore`: TCK AC-41..44 + AC-45 + RemoveExpired PASS on `postgres:16-alpine`.
- `mongostore`, `etcdstore`: TCK + AC-45 expiration PASS on real containers.

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
- `ai-context/done/q6-pgstore-read-only-row-leak.md` — **ruled and done**
  (ADR 0010): the insert moves inside the locking transaction and runs only
  when the callback writes; both original candidates rejected. Implemented,
  black-box verified: `ai-context/done/pgstore-no-row-for-read-only-miss-ac48.md`
  (AC-48 = storage footprint tracks writes, not reads).
- `ai-context/done/q8-lockstore-do-error-contract.md` — **ruled and done**
  (ADR 0009): `Do` must return `fn`'s error unmodified or `%w`-wrapped, never
  replaced, flattened, or swallowed. Doc-only; TCK enforcement deferred.
  Implemented, black-box verified: `ai-context/done/lockstore-do-godoc-contract.md`.
- `ai-context/tasks/q9-errstore-contract-ac.md` — assign an AC id to the
  `ErrStore` contract the 8 tests in `core/storeerr_test.go` currently test
  without one; unblocked by Q8 — the decode-vs-outage clause stays a core
  obligation, not a per-backend TCK one.

- `ai-context/tasks/plans-md-vs-changelog.md` — **deferred, unruled**: whether
  this file survives as a tracker or becomes a versioned changelog now that
  `tasks/`/`done/` carry work status (raised in PR #7 review).

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
- _q6_: pgstore read-only row leak ruled (ADR 0010) and implemented —
  placeholder insert stops being committed up front; it moves inside the
  locking transaction and runs only when the callback writes, so a read of
  an absent key persists nothing (parity with Redis/memstore). Short-
  `expires_at` and existence-probe candidates rejected. `TestAC48_...`
  watched failing against the pre-fix code first; black-box verified via
  `examples/k6/run-sandbox.sh postgres`.
- _tester-redefined_: the tester's "end-to-end" gate was running `go test`
  and reading source/DB state directly — a second unit/integration pass, not
  an independent behavioral check (docs/adr/0011). Replaced with a black-box
  k6 sandbox (`examples/k6/`) that runs the real compiled `examples/redis`/
  `examples/postgres` binary against a real backend container and asserts
  only on observed HTTP status codes. `tester.md` now has only `Bash`, no
  `Read`/`Grep`/`Glob` (policy boundary, not a hard technical one). Q5, Q6,
  and Q8 re-gated for real against the new sandbox after this landed — their
  original go-test-based "e2e" passes did not count.
- _q8_: `LockStore.Do` error contract ruled (ADR 0009) and implemented —
  must propagate `fn`'s error unmodified or `%w`-wrapped (never
  `%v`-flattened or swallowed); doc-only, TCK enforcement deferred until a
  second `LockStore` exists (`tck.Run` only sees a `ProxyManager`, so it
  can't plant the corrupt bytes needed to trip `fn`). Relaxing core's guard
  to ignore `Do`'s return rejected: breaks on a store that retries `fn`
  internally. `pgstore` already complies. Q9 unblocked. go.mod bumped to
  1.26.5 (all four modules) to match the already-verified toolchain.
- _scope-mongo-etcd_: both new-backend tasks scoped for parallel dev —
  `mongostore` as a `CASStore` over conditional `ReplaceOne` (`_id`=key,
  `state` BinData, `expiresAt`; TTL index for reclamation only, expiry
  enforced in the query predicate; `IndexModels`/`EnsureIndexes` as the
  `SchemaSQL` analogue), `etcdstore` as a `CASStore` over `Txn` comparing
  `clientv3.Value` — ModRevision rejected because `CASStore` passes bytes,
  not a revision — with per-write leases as authoritative expiry and no
  setup step. ADR 0012 records why two backends need an app-level expiry
  check and two do not.
- _mongo-etcd-done_: both backends implemented and gated — `mongostore` and
  `etcdstore` pass the TCK against real containers and the black-box k6
  sandbox (`examples/k6/run-sandbox.sh mongodb|etcd`); tasks moved to
  `ai-context/done/`. Client docs: `docs/client/mongodb.md`, `docs/client/etcd.md`.
  Remaining new-backend backlog: `ai-context/tasks/vendor-hazelcast-store.md`.
- _pr7-process_: task files now require a **Test scenarios** section written by
  the Architect — black-box cases for the tester, unit/integration cases for
  Dev, so the two stop deriving coverage independently (ADR 0013; convention in
  `ai-context/tasks/README.md`, pointers added to all three agent files).
  Letting the tester author its own scenarios rejected (no `Read` tools per ADR
  0011, and a scenario invented by the run that grades it isn't independent).
  Whether `PLANS.md` should become a versioned changelog is deferred, not
  answered — captured as `ai-context/tasks/plans-md-vs-changelog.md`. ADR 0011's
  k6 path corrected to `e2e/k6/` after the sandbox moved out of `examples/`.
- _roadmap_: three read-only audits (code, docs, ecosystem) against 5f45eb8
  landed as task files, ranked in `ai-context/tasks/next-roadmap.md` — that
  file, not this one, is now where "what is queued" is read. Two rulings
  recorded: Go 1.23 becomes the single project version (a separate dev pin
  rejected; the highest dependency-forced floor is `etcdstore` at go1.22, and
  the earlier "1.24" figure was a `go.work` measurement artifact), and CI moves
  to GitHub Actions with no test changes (runner Docker satisfies
  testcontainers; a rewrite onto GHA `services:` rejected as unnecessary). Both
  ADRs are written when the changes are applied, not now. Four tier-1
  correctness items scoped as their own files (`initialTokens` validation, etcd
  lease leak, CAS retry hardening, ctx cancellation); nothing implemented this
  round. Whether this file survives as a tracker remains open
  (`ai-context/tasks/plans-md-vs-changelog.md`) — this entry follows the
  established format and does not prejudge that ruling.
- _roadmap-review_: independent architect review of the above before commit.
  AC ids pinned rather than left to pickup — **AC-49** `initialTokens`
  validation, **AC-50** ctx cancellation on the distributed path, and *no* id
  for CAS retry pacing (implementation property; the exhaustion contract stays
  with `ai-context/tasks/q9-errstore-contract-ac.md`), since three files all
  claiming "the next free id" collide when two are worked in parallel. Two
  corrections: the etcd lease behavior is not merely undocumented but
  contradicted (`docs/client/etcd.md:22` says "successful write"; the grant is
  per attempt), and the ctx task's black-box surface exists but is a race the
  sandbox cannot control — reason restated, scenario still declined.
- _roadmap-reranked_: the repo owner answered the four direction questions and
  `ai-context/tasks/next-roadmap.md` was re-ranked against them. This is a
  **portfolio / reference implementation**, not a near-term release, so
  `publish-v0.1.0.md` is dormant (kept: the `replace` + `require core v0.0.0`
  trap is invisible under `go.work` and would break the first outside
  `go get`). **Cheap pluggability is the product**, so widening the TCK and
  forwarding `core.ProxyOption` from every backend move to tier 1, and `core`
  stays vendor-agnostic — round-trip reduction belongs in the command layer
  (new row 19, unscoped). **Multi-bandwidth is planned for v0.2**, so the
  config fingerprint folded into one future codec-format change (row 17) with
  a prerequisite row 18: state that the codec byte layout is unstable before
  v1.0 *before* breaking it. `examples/` are demos users will copy, so the
  error-body IP leak and missing `Retry-After` move to tier 2. Row numbers are
  now stable ids, not positions — the Tier column carries the ranking, so a
  re-rank stops invalidating every "row N" cross-reference. The Go-version and
  CI files had their "Ruling (repo owner)" headers corrected to separate the
  owner's direction (one version; yes to GitHub CI) from the Architect's
  derivation (1.23 from the measured floors; the job design).
- _ac50-error-shape_: ruled the error shape `core-ctx-cancellation.md` was
  deferring — a cancelled or expired context returns the **bare** context
  error, not `ErrStore` (ADR 0015, a carve-out from AC-51 clause 1 and a
  narrowing of ADR 0002), with `storeErr` (`core/command.go:17`) as the single
  choke point so a driver-noticed and a guard-noticed cancellation cannot
  differ by a race; rejected the `ErrStore`-wrapped variant because the
  documented check is `ErrStore` first, which routes every client disconnect
  into the outage branch. **Contested and unresolved:** open PR #12 already
  implements the opposite, consistently, and both cannot merge — the owner
  picks. Also rejected merging roadmap rows 5 and 6: same loop, but a
  correctness fix should not wait on a backoff-tuning review; row 6 settles
  the shared line for row 5 instead (interruptible sleep, bare context error).
  Kept the `MemStore` ctx guards, demoted to hygiene with the warning that an
  AC-50 test driven through `MemStore` passes with the executor guard missing.
- _ac52-vendor-ctx_: new task `ai-context/tasks/tck-vendor-ctx-conformance.md`
  (roadmap row 20, tier 1). "Every backend honours ctx" is currently an
  assertion in prose; AC-50's executor guard makes it untestable through
  `Bucket`, so the case has to run against the `CASStore`/`LockStore` value.
  Ruled a sibling of row 7 rather than part of it — different boundary,
  different entry-point signature — and ruled it reachable via `export_test.go`
  rather than by exporting `NewStore` from `redisstore`/`etcdstore`, since
  widening the public API is not a test task's call. It is unblocked by the
  error-shape dispute: `ErrStore` is applied above the store.
- _ctx-rulings_: the owner ratified ADR 0015 — bare `ctx.Err()`, not
  `ErrStore`. The two arguments that settled it are now in the ADR and were not
  in the original reasoning: `storeErr`'s double `%w` means the rejected option
  lost no information either, so the question was only whether cancellation is
  *classified* as a store failure; and `database/sql`, which fronts pluggable
  drivers behind one interface exactly as `core` does, returns a bare
  `ctx.Err()` from its pre-flight checks (`sql.go:1327,1396,2249,2297`, Go
  1.25.6 tree). Consequences recorded for other branches, not applied here:
  PR #12's guards become `return ctx.Err()` and AC-51 clause 1 grows the
  carve-out. Migration note in the ADR — a caller checking only `ErrStore` now
  falls through on cancellation, which is intended.
- _ac52-deferred_: `tck-vendor-ctx-conformance.md` was filed on a false
  premise. A driver audit against this repo's own pins found all four already
  return `errors.Is(err, context.Canceled)` — etcd included, via `ContextError`
  substituting `ctx.Err()` for a gRPC status code, which was the predicted
  failure. Reframed from bug hunt to regression guard, dropped to tier 2 and
  deferred pending the etcd-scope question. The real gap stands: no test
  anywhere exercises a cancelled context against a real backend.
- _ac54-reservation-refund_: new task
  `ai-context/tasks/core-blocking-reservation-refund.md` (AC-54, roadmap row
  22, tier 1). `reserve` persists the deduction before the sleep, so a cancelled
  `Consume` charges tokens to nobody; `core/blocking.go:22-23` documents that as
  intended and the owner ruled it is not. Compensating release through the
  executor, best-effort but reported: on refund failure the caller gets
  `ctx.Err()` wrapped with a new `ErrReservationNotReleased` sentinel, and
  deliberately **not** `ErrStore` — tagging a departed client as a store
  failure is what ADR 0015 rejected. Known ceiling named and not solved: a CAS
  that commits but loses its reply still over-counts.
