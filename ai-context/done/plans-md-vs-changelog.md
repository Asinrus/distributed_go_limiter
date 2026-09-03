# Does `ai-context/PLANS.md` still earn its place?

**Origin:** PR #7 review comment on `ai-context/PLANS.md:33` — "do we really
need this tracker if we have /done /tasks, better transform it into the
changelog and put a specific version in each change."

**Problem:** `PLANS.md` has been narrowed twice already — decisions moved to
`docs/adr/` and per-question tracking moved to task files
(`docs/adr/0007-tasks-done-workflow.md`). What's left is a phase table, a
verification section, and a long free-form changelog. Two of those three now
duplicate something else: the phase table restates what `ai-context/done/`
already shows, and the "how to run" block restates `CLAUDE.md`. The changelog
is the part with no other home — but its entries are keyed to sessions, not to
anything a reader can locate (no version, no tag, no commit), so it can't be
matched against a release either.

The reviewer's proposal is to keep only the changelog and version each entry.
That is one option, not the ruling — the task is to decide, with the
alternatives written down.

**In scope**
- Decide whether `PLANS.md` survives, and in what form. At least consider:
  keep as-is; reduce to a versioned changelog only; delete it and move the
  changelog to `CHANGELOG.md` at the repo root under Keep a Changelog +
  semver, leaving `ai-context/` purely process.
- If a version is attached to entries, define what a version *means* here —
  five independently versioned Go modules (`core`, `redisstore`, `pgstore`,
  `mongostore`, `etcdstore`) with no tags today, so "0.3.0" needs a stated
  scope before it means anything.
- Decide what happens to the non-changelog content that would be dropped
  (phase table, verification block, "Open work" list) — folded elsewhere or
  deleted as duplication, one line of rationale each.
- Update `.claude/agents/architect.md`, `ai-context/README.md`, and `CLAUDE.md`
  wherever they point at `PLANS.md`, if the outcome moves or removes it.

**Out of scope**
- Actually tagging releases or setting up release tooling.
- Any change to the `tasks/` → `done/` lifecycle itself; that one works.

**Done looks like:** an ADR recording the ruling *and the rejected options with
their reasons* (this question will otherwise be reopened), the file
restructured or deleted to match, and no dangling references to it anywhere in
the repo (`grep -rn PLANS.md` comes back consistent with the ruling).

**Test scenarios:** no black-box surface — process/docs only, no Go code and no
observable behavior change, so Gate 2 is satisfied by Gate 1
(`docs/adr/0013-architect-scopes-test-coverage.md`). Dev is not involved; this
is Architect work end to end.

## Ruling — `PLANS.md` deleted, no replacement changelog file

Recorded in `docs/adr/0016-retire-plans-md.md`.

**What survives, and where:**
- What changed, when, and why it was changed — `git log` on `main` (the
  messages already say this; several already do it well) plus `docs/adr/` for
  the decisions that clear ADR 0007's bar.
- What is done — `ai-context/done/`.
- What is queued and in what order — `ai-context/tasks/next-roadmap.md`.
- What is in flight — the task files still sitting in `ai-context/tasks/`.

No single file replaces `PLANS.md` as "the tracker" — the four sources above,
each already authoritative for its own slice, are the tracker collectively.
This is the same convention already in force elsewhere in this repo (task
files don't restate `done/`'s existence; ADRs don't restate the task file
that raised them) and matches the general rule against tracking in prose what
version control, or another file already in the repo, already answers.

**Alternatives considered and rejected:**

1. **Keep as-is.** Rejected — it had already been agreed twice (via the two
   prior narrowings) that most of its remaining content is duplication; the
   phase table restates `ai-context/done/` + `next-roadmap.md`, and the
   "how to run" block restates `CLAUDE.md`. Keeping a hand-edited file whose
   sources of truth are elsewhere is a standing drift risk with no upside.
2. **Reduce to a versioned changelog only** (the PR #7 reviewer's proposal).
   Rejected — a version needs a stated scope before "0.3.0" means anything,
   and this repo has none: five independently-versioned modules (`core`,
   `redisstore`, `pgstore`, `mongostore`, `etcdstore`), zero git tags, and no
   root module for them to roll up to (root is `go.work`-only). Inventing a
   repo-wide version number solely to stamp changelog entries is a scheme
   built to serve the changelog, not one that tracks anything real, since no
   release consumes it.
3. **Delete `PLANS.md`, move the changelog to a root `CHANGELOG.md`**
   (Keep a Changelog + semver, `ai-context/` left purely process). Rejected
   for now, not forever — Keep a Changelog's "Unreleased" section sidesteps
   the versioning problem in (2), so this is the credible fallback if a
   changelog is wanted again. But the repo owner has ruled this a portfolio /
   reference implementation with no near-term release
   (`ai-context/tasks/next-roadmap.md`), so there is no consumer to serve, and
   the same history is already captured — unglamorously but completely — by
   `git log`. Adding the file now is release scaffolding for a release that
   isn't planned, the same reasoning that already keeps `publish-v0.1.0.md`
   dormant. Revisit if/when a version is actually cut.

**Queue-index question, since `next-roadmap.md`'s own preamble defers to
this file on it:** yes, a queue index belongs in `ai-context/tasks/` beside
the per-item files. `next-roadmap.md` is not itself a work item (its own
preamble already says so — it never moves to `done/` and Test scenarios don't
apply to it) and was never really competing with `PLANS.md`; the "open work"
section `PLANS.md` used to carry is now fully superseded by it. Nothing
further changes about how `next-roadmap.md` works.

**Deferred deliberately** no longer applies — raised during PR #7, parked so
the backend work could land first, and ruled in this session.

**Re-tested against PR #19 (`31f757a`), which added 54 lines to `PLANS.md`
after the ruling.** Ruling unchanged; the reasoning is in
`docs/adr/0016-retire-plans-md.md`'s "PR #19" section, including the mapping
of those 54 lines onto ADR 0015 and the two task files added in the same
commit.
