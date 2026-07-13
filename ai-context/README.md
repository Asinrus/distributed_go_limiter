# ai-context

Entry point for AI agents / contributors working on this repo: the process, roles,
and progress. This folder is **committed**. The detailed design & investigation
documents are **local-only** under `docs/internal/` (gitignored) — if they're
missing from a fresh clone, they can be regenerated from the reference library
(`../bucket4j`) or requested from the person who has them.

## Project in one line
A distributed **token-bucket** rate limiter (port of bucket4j). `core` owns the
algorithm + `Bucket` API; storage providers (local, Redis, Postgres) implement a
tiny `Store`. **Primary goal: swap the provider without changing rate-limiting
logic** — the same conformance suite (`core/tck`) passes against all three.

## What lives where
| Path | Tracked | Contents |
|------|---------|----------|
| `ai-context/PLANS.md` | ✅ committed | Progress tracker (phases, status, decisions). **Architect-owned — only the architect edits it.** |
| `ai-context/README.md` | ✅ committed | This guide. |
| `docs/internal/project/` | ⛔ local | `ARCHITECTURE.md` (mermaid + provider-swap examples), `EXT_REDIS.md`, `EXT_POSTGRES.md`, `ACCEPTANCE_CRITERIA.md`, `QUESTIONS_TO_ARCHITECT.md`, `problem_reports/`. |
| `docs/internal/bucket4j/` | ⛔ local | Investigation of the reference library (algorithm, backends, module/test layout, provider-swap abstraction). |
| `CLAUDE.md` | ✅ committed | Code map + build/test commands. |

## Roles & process
- **Architect** — owns `PLANS.md` and `docs/internal/project/`. May dispatch an
  **Investigator** (findings → `docs/internal/bucket4j/`) before finalizing a design decision.
- **Dev** — implements TDD against the acceptance criteria
  (`docs/internal/project/ACCEPTANCE_CRITERIA.md`). Every criterion has a stable ID
  (AC-11 … AC-46) referenced from the test name (`TestAC12_...`); the shared TCK
  (`core/tck`) is the executable form and runs against local + Redis + Postgres.
- **Failure-handling loop** — on any verification failure (build error, failing
  test, unmet criterion): the Investigator writes
  `docs/internal/project/problem_reports/<phase>-<n>.md` (symptom, failing AC id,
  root cause, defect-vs-spec-gap) and triggers Dev (code fix) or Architect
  (spec/design fix). Loop until the criteria pass; Architect records the outcome in `PLANS.md`.

## Key decisions (v1)
- **Single bandwidth per bucket.** Deferred: multi-bandwidth, async API, verbose
  diagnostics, optimization/batching, config replacement + versioning, grid providers.
- `Bucket` methods take `ctx` and return `error` (local ignores both; distributed
  surfaces I/O + cancellation) — calling code stays identical across backends.
- Config is supplied at bucket-build time and **not stored**; nodes sharing a key
  must use matching config.
- Postgres v1: `SELECT … FOR UPDATE` only (advisory-lock strategy deferred).

## Status
All six phases done and **verified** (Go 1.26.5; core `go test -race` + Redis/Postgres
testcontainers TCK all green). See `PLANS.md` for the phase table and the exact
run commands. Build/test quick reference is in `../CLAUDE.md`.
