# ai-context

Entry point for AI agents / contributors working on this repo: the process, roles,
and progress. This folder is **committed**, alongside `docs/adr/` (ratified
decisions) — see below for why that replaced the old local-only `docs/internal/`.

## Project in one line
A distributed **token-bucket** rate limiter (port of bucket4j). `core` owns the
algorithm + `Bucket` API; storage providers (local, Redis, Postgres) implement a
tiny `Store`. **Primary goal: swap the provider without changing rate-limiting
logic** — the same conformance suite (`core/storetest`) passes against all three.

## What lives where
| Path | Contents |
|------|----------|
| `ai-context/README.md` | This guide. |
| `ai-context/tasks/` | One file per work item (dev or investigation), created by the Architect — including the **Test scenarios** section Dev and the Tester both work from (`ai-context/tasks/README.md`). Moves to `ai-context/done/` only when the tester agent confirms both `/verify` and an end-to-end run pass. `next-roadmap.md` in the same folder is the standing queue index (not itself a work item, doesn't move to `done/`). |
| `ai-context/done/` | Completed, tester-verified task files. |
| `docs/adr/` | One file per ratified decision that's hard to reverse, surprising without context, and the result of a real trade-off — see `docs/adr/0007-tasks-done-workflow.md`. |
| `docs/client/` | User-facing backend docs (`redis.md`, `postgres.md`). |
| `.claude/agents/` | `architect.md`, `dev.md`, `investigator.md`, `tester.md` — the roles below, in executable form. |
| `.claude/commands/` | `/verify` — full suite across every workspace module; dispatches the investigator on failure. |
| `CLAUDE.md` | Code map + build/test commands. **Dev-owned** — it follows the code, so Dev updates it; the Architect opens a task in `ai-context/tasks/` rather than editing it silently. |

`docs/internal/` no longer exists — it was gitignored, local-only, and absent
from this workspace entirely. Its role is split between `docs/adr/`
(committed decisions) and `ai-context/tasks/` (committed work items,
including what used to be `problem_reports/`).

## Roles & process
The four roles are dispatchable subagents; **their agent files are the
authoritative definition** — read those, not a paraphrase here.

| Role | Model | Definition | Owns |
|------|-------|-----------|------|
| Architect | opus | `.claude/agents/architect.md` | `docs/adr/`, `ai-context/tasks/` (creates) |
| Dev | sonnet | `.claude/agents/dev.md` | Go source, tests, `CLAUDE.md` |
| Investigator | sonnet | `.claude/agents/investigator.md` | Writes findings into the task file it's given |
| Tester | haiku | `.claude/agents/tester.md` | Moves `ai-context/tasks/*` → `ai-context/done/*` — the only role that does |

**Failure-handling loop** (the part no single agent file encodes) — on any
verification failure (build error, failing test, unmet criterion): the
Architect opens (or points at) a task in `ai-context/tasks/`; the Investigator
appends findings to it (symptom, failing AC id, root cause, defect-vs-spec-gap)
and that routing triggers Dev (code fix) or Architect (spec/design fix). Loop
until the criteria pass, then the Tester independently reruns `/verify` plus a
small end-to-end scenario and only then moves the file to `ai-context/done/`.
`/verify` (`.claude/commands/verify.md`) runs the suite and dispatches the
Investigator on failure automatically, so the loop starts without anyone
remembering to start it.

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
There is no single progress-tracker file (`ai-context/PLANS.md` was retired, see
`docs/adr/0016-retire-plans-md.md`). What changed and why: `git log` on `main` plus
`docs/adr/`. What's done: `ai-context/done/`. What's queued: `ai-context/tasks/next-roadmap.md`.
Build/test quick reference is in `../CLAUDE.md`; `/verify` runs the suite.
