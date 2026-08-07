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
| `.claude/agents/` | ✅ committed | `architect.md`, `dev.md`, `investigator.md` — the roles below, in executable form. |
| `.claude/commands/` | ✅ committed | `/verify` — full suite across the three modules; dispatches the investigator on failure. |
| `CLAUDE.md` | ✅ committed | Code map + build/test commands. **Dev-owned** — it follows the code, so Dev updates it; the Architect files a question in `QUESTIONS_TO_ARCHITECT.md` rather than editing it silently. |

## Roles & process
The three roles are dispatchable subagents; **their agent files are the
authoritative definition** — read those, not a paraphrase here.

| Role | Definition | Owns |
|------|-----------|------|
| Architect | `.claude/agents/architect.md` | `ai-context/`, `docs/internal/project/` |
| Dev | `.claude/agents/dev.md` | Go source, tests, `CLAUDE.md` |
| Investigator | `.claude/agents/investigator.md` | `docs/internal/project/problem_reports/` |

**Failure-handling loop** (the part no single agent file encodes) — on any
verification failure (build error, failing test, unmet criterion): the
Investigator writes `docs/internal/project/problem_reports/<phase>-<n>.md`
(symptom, failing AC id, root cause, defect-vs-spec-gap) and that routing triggers
Dev (code fix) or Architect (spec/design fix). Loop until the criteria pass;
Architect records the outcome in `PLANS.md`. `/verify`
(`.claude/commands/verify.md`) runs the suite and dispatches the Investigator on
failure automatically, so the loop starts without anyone remembering to start it.

## Key decisions (v1)
- **Single bandwidth per bucket.** Deferred: multi-bandwidth, async API, verbose
  diagnostics, optimization/batching, config replacement + versioning, grid providers.
- `Bucket` methods take `ctx` and return `error` (local ignores both; distributed
  surfaces I/O + cancellation) — calling code stays identical across backends.
- Config is supplied at bucket-build time and **not stored**; nodes sharing a key
  must use matching config.
- **`core` is protocol-agnostic.** No HTTP (or gRPC, or any transport) inside the
  library — an adapter belongs in the caller. Store failures are made *identifiable*
  instead: they wrap `core.ErrStore`, and the caller decides fail-open vs
  fail-closed with `errors.Is(err, core.ErrStore)`. Core never picks a status code,
  a response shape, or a degradation policy.
- Postgres v1: `SELECT … FOR UPDATE` only (advisory-lock strategy deferred).

## Status
Phases 0–6 done and **verified** (Go 1.26.5; core `go test -race` + Redis/Postgres
testcontainers TCK all green). Phase 7 (`core/httplimit`) was **removed** — core is
protocol-agnostic; it was replaced by `core.ErrStore`, whose tests pass but which no
acceptance criterion covers yet (open Q9). See `PLANS.md` for the phase table and the exact run commands. Build/test quick reference is in `../CLAUDE.md`.
