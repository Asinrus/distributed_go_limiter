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

## PR #19 added 54 more lines to `PLANS.md` after this was ruled

Sequencing, stated so a later reader does not have to wonder whether it was
noticed: this ruling was written against `89b06ff`; PR #19 (`31f757a`) then
merged to `main` and added an ADR-index line plus five changelog entries to
`PLANS.md`. **The ruling stands, and the delete keeps nothing back.** Each
addition was checked against its own source first:

| Added in PR #19 | Where the same content already lives |
|---|---|
| ADR index gains `cancellation is not a store failure (0015…)` | `docs/adr/0015-cancellation-is-not-a-store-failure.md` — and the `docs/adr/` listing, which *is* the index |
| `_ac50-error-shape_`, `_ctx-rulings_` | ADR 0015 (the ruling, `storeErr` as the choke point, the `database/sql` precedent, the migration note) and `ai-context/tasks/core-ctx-cancellation.md:34,67,95` (the rows 5/6 split, the `MemStore` hygiene demotion) |
| `_ac52-vendor-ctx_`, `_ac52-deferred_` | `ai-context/tasks/tck-vendor-ctx-conformance.md:3,13,61,80` (the deferral, the tier drop, the driver audit against pinned versions, the `export_test.go` and row-7-sibling rulings) |
| `_ac54-reservation-refund_` | `ai-context/tasks/core-blocking-reservation-refund.md:34,39,57,69` (the `WithoutCancel` write, the capacity clamp, the `ErrReservationNotReleased` shape, the known ceiling) |

Every entry is a shorter paraphrase of a file added in the same commit. The
task files carry `path:line` evidence, AC ids and Test scenarios; the
changelog entries carry none of those, so the paraphrase is the lossy copy.

The merge is evidence *for* the ruling rather than against it. Inside that one
PR, ADR 0015 was first written as contested and then ratified by the owner in
a later commit — and `PLANS.md`'s index line still read
`(0015, contested — see below)` (`31f757a:ai-context/PLANS.md:21`) while the
ADR it indexes read `## Ratified`
(`docs/adr/0015-cancellation-is-not-a-store-failure.md:54`), with a changelog
entry further down recording the ratification correctly. One PR, one
hand-edited file, and the summary already contradicted the source it
summarises in one place while agreeing with it in another. That is the drift
alternative (1) predicts, observed rather than hypothesised.

What the merge does show is that a ruling on an unmerged branch does not reach
a concurrent author: PR #19 was written from a worktree where this branch was
invisible, so the Architect role will keep writing to `PLANS.md` until the
role files on `main` stop naming it. That is why the four `.claude/agents/*.md`
edits are part of this change and not a follow-up.

This is the third and final narrowing of the same file, so it supersedes ADR
0007's clause that "`ai-context/PLANS.md` keeps the high-level phase table and
changelog" (`docs/adr/0007-tasks-done-workflow.md:12`); 0007 gets a pointer
rather than a silent contradiction.

## Consequence

`ai-context/tasks/next-roadmap.md` is confirmed as the standing queue index
beside `ai-context/tasks/`'s per-item files — it isn't a work item itself (its
own preamble already says so) and doesn't move to `done/`; it simply isn't
competing with anything anymore now that `PLANS.md`'s "open work" section is
gone. `ai-context/README.md`, `.claude/agents/architect.md`, `.claude/agents/dev.md`,
`.claude/agents/investigator.md`, and `.claude/agents/tester.md` are updated
to stop pointing at `PLANS.md`. `CLAUDE.md:19` and the root `README.md:60,66`
still point at it — both are Dev-owned and were held by another agent, so they
are folded into `ai-context/tasks/next-roadmap.md` row 11 rather than edited
here. Task files already in `ai-context/done/` keep their `PLANS.md`
references: they are a record of what was true when the work was gated, and
`git log -p -- ai-context/PLANS.md` still resolves after the delete.

## Alternative (3) is now in force — its trigger fired

Alternative (3) above (a root `CHANGELOG.md`) was rejected "for now, not
forever", with the condition named: *revisit if/when a version is actually
cut*. A v0.1.0 is being tagged
(`docs/adr/0019-v0-1-0-is-five-lockstep-modules-under-apache-2-0.md`), so the
condition is met and the file now exists —
`docs/adr/0020-changelog-starts-at-v0-1-0.md`.

This ADR's "no replacement changelog file" means no replacement *progress
tracker*, which is what `PLANS.md` was. The four-way split above is unchanged;
nothing moves back into a single file, and the new file carries only what a
consumer of a tagged module cannot get anywhere else.
