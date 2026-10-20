# T10 — `CHANGELOG.md`, seeded at v0.1.0

**For:** Dev. One new file, no Go code. **Blocks** `publish-v0.1.0.md` step 5.

**Ruled in** `docs/adr/0020-changelog-starts-at-v0-1-0.md`. The question was
whether `docs/adr/0016-retire-plans-md.md` ("no replacement changelog file")
already settled this. It does not: 0016 was about *internal progress
tracking*, and it rejected a consumer changelog "for now, not forever" on two
grounds that have both expired — there was no version scope (ADR 0019 gives
one) and no release (there is one now). 0016 named the trigger itself:
*revisit if/when a version is actually cut.* It has a pointer to 0020 now, so
the ADR set does not contradict itself.

## In scope

- Root `CHANGELOG.md`, Keep a Changelog format, semver, with `Unreleased` and
  `0.1.0` sections. One file for all five modules, because this is one repo
  with one shared contract — **not** because they share one number
  (`docs/adr/0021-modules-version-independently-after-v0-1-0.md`).
- The `0.1.0` entry. It is the first release, so it is a *what this is* entry,
  not a diff against nothing: the `Bucket` API, single bandwidth per bucket,
  blocking consume, the four backends, the shared conformance suite as the
  provider-swap proof. Short — five or six bullets, not a feature tour.
- A line in `0.1.0` for the thing a consumer must know before they depend on
  it: the stored byte layout is not part of the API and changes before v1.0.
  Reuse `t09`'s wording; do not write a third phrasing of it.
- A short note at the top of the file saying what versioning scheme is in
  force, pointing at
  `docs/adr/0021-modules-version-independently-after-v0-1-0.md`: the five
  library modules all start at `v0.1.0` and are tagged independently after
  that, `examples` is not published, and an entry names the module and version
  it lands in when that is not obvious. Nobody should have to infer the scheme
  from the entries, and they must not infer lockstep.
- The `0.1.0` date is the date the tags are cut — `publish-v0.1.0.md` step 5.
  Leave it as the literal string `UNRELEASED` until then rather than guessing
  a date that will be wrong.

## What stays out of it

This is the half of the ticket most likely to go wrong, because the temptation
is to write the project's history into it:

- No session-keyed narrative, no AC ids, no task-file or PR references, no
  "refactored X". ADR 0016 rejected `PLANS.md` for exactly this content and
  that rejection stands.
- No restating `ai-context/done/`, `docs/adr/` or `git log`. If a line does not
  change what a caller writes or what they must know before upgrading, it does
  not belong.
- Not a release-automation ticket. Hand-written, four lines a release.

## Out of scope

- GitHub release notes. They should carry the same text when the repo goes
  public, which is after this round (ADR 0019); nothing to do now.
- Changing anything about the internal tracker split (`git log` / `docs/adr/`
  / `ai-context/done/` / `next-roadmap.md`). Unchanged.
- Per-module changelogs.

**Done looks like:** a root `CHANGELOG.md` a stranger can read in thirty
seconds and come away knowing what v0.1.0 gives them and what will break
next; `grep -c '' CHANGELOG.md` comfortably under 60.

## Test scenarios

**Black-box (tester, Gate 2):** no black-box surface — one new markdown file,
no Go code, no behaviour change. Gate 2 is satisfied by Gate 1
(`docs/adr/0013-architect-scopes-test-coverage.md`).

**Unit / integration (Dev):** none, and no AC id. The one check that matters is
a review check, not a test: every claim in the `0.1.0` entry must be true of
the tree at the tagged commit — specifically the backend count (four), the
byte-layout caveat matching `core/codec.go:13-14`, and the version scheme
matching ADR 0019. A changelog that overstates the first release is the
failure mode here, and no test catches it.
