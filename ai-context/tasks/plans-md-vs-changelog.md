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

**Deferred deliberately.** Raised during PR #7 and parked by the repo owner so
the backend work could land first. Not blocking anything.
