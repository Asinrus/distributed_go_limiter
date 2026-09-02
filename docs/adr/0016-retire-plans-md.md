# Retire `ai-context/PLANS.md`; no replacement changelog file

`PLANS.md` had already been narrowed twice: ratified decisions moved to
`docs/adr/` (ADR 0007), per-question tracking moved to `ai-context/tasks/`.
What was left — a phase table, a "how to run" block, an "open work" list, and
a free-form changelog — all duplicate something else that now exists:

- The phase table restates what `ai-context/done/` and
  `ai-context/tasks/next-roadmap.md` already show (done work vs. queued work).
- The "how to run" block restates `CLAUDE.md`'s build/test section.
- The "open work" list restates `ai-context/tasks/` itself and, since PR #7,
  `next-roadmap.md` specifically.
- The changelog is genuinely narrative and has no other single home — but its
  entries are keyed to agent sessions, not to anything a reader can locate
  (no tag, no version, no commit range), so it can't be matched against a
  release, and it silently re-narrates what `git log` and the ADRs already
  say happened and why.

## Decision

Delete `ai-context/PLANS.md`. Do not replace it with a root `CHANGELOG.md` or
any other single file. Going forward:

- **What changed, when** — `git log` on `main` (commit messages already state
  what changed and why; several already do this well, e.g. 90240ab, b87e5ee).
- **Why a decision was made** — `docs/adr/`, for the decisions that clear the
  bar in `docs/adr/0007-tasks-done-workflow.md`.
- **What is done** — `ai-context/done/`.
- **What is queued and in what order** — `ai-context/tasks/next-roadmap.md`.
- **What is in flight right now** — `ai-context/tasks/` (files not yet moved).

No file is now the single "progress tracker"; the tracker is the union of the
four above, each already the authoritative source for its slice. This matches
the project's own convention against tracking in prose what version control
(or another file already in the repo) already answers.

## Alternatives considered

1. **Keep `PLANS.md` as-is.** Rejected: it was already agreed twice over that
   most of its content is duplication (that's *why* it was narrowed twice);
   keeping it as-is preserves a chronic drift risk (`PLANS.md` is hand-edited,
   the sources it duplicates are not) for a file most of whose sections are
   answered elsewhere.
2. **Reduce `PLANS.md` to a versioned changelog only** (the PR #7 reviewer's
   proposal). Rejected: a version needs a stated scope before "0.3.0" means
   anything, and this repo has none — five independently-versioned Go modules
   (`core`, `redisstore`, `pgstore`, `mongostore`, `etcdstore`) with zero git
   tags today, and no single top-level version they roll up to (there is no
   root module; root is `go.work`-only). Inventing a repo-wide version number
   to stamp on changelog entries would be a versioning scheme built solely to
   satisfy the changelog, not one that tracks anything real (no release
   consumes it). If that changes — a tag gets cut for a real reason — a
   changelog can be started fresh at that point with a version that means
   something, rather than back-filling meaning onto entries that predate it.
3. **Delete `PLANS.md`, move the changelog to a root `CHANGELOG.md`** (Keep a
   Changelog + semver, `ai-context/` left purely process). Rejected for now,
   not forever: Keep a Changelog's "Unreleased" section sidesteps the
   versioning problem in (2), so this is the credible fallback if a changelog
   is ever wanted again — but the repo owner has ruled this a portfolio /
   reference implementation with no near-term release
   (`ai-context/tasks/next-roadmap.md`), so there is no consumer for a
   user-facing changelog to serve, and internal session history is already
   captured, unglamorously but completely, by `git log`. Adding the file now
   is scaffolding for a release that isn't planned; the task list already
   flags `publish-v0.1.0.md` as dormant for the same reason. Revisit if/when a
   version is actually cut.

## Consequence

`ai-context/tasks/next-roadmap.md` is confirmed as the standing queue index
beside `ai-context/tasks/`'s per-item files — it isn't a work item itself (its
own preamble already says so) and doesn't move to `done/`; it simply isn't
competing with anything anymore now that `PLANS.md`'s "open work" section is
gone. `ai-context/README.md`, `.claude/agents/architect.md`, `.claude/agents/dev.md`,
`.claude/agents/investigator.md`, and `.claude/agents/tester.md` are updated
to stop pointing at `PLANS.md`. `CLAUDE.md` and the root `README.md` still
have one stale pointer each (out of scope for this change — another agent
owns both files concurrently); tracked as a one-line follow-up.
