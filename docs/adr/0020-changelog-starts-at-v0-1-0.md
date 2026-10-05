# A consumer-facing `CHANGELOG.md` starts at v0.1.0

`docs/adr/0016-retire-plans-md.md` deleted `ai-context/PLANS.md` and ruled "no
replacement changelog file". That ruling stands for what it was about —
**internal progress tracking**. It does not settle release notes for
consumers, and it said so: alternative (3) (a root `CHANGELOG.md`, Keep a
Changelog + semver) was rejected **"for now, not forever"**, on two stated
grounds, with an explicit trigger — *"revisit if/when a version is actually
cut."*

Both grounds have now expired, which is why this is a new ADR and not a
re-litigation:

- *"A version needs a stated scope before 0.3.0 means anything, and this repo
  has none."* It has one now:
  `docs/adr/0019-v0-1-0-is-five-lockstep-modules-under-apache-2-0.md` puts the
  same number on all five library modules, so one entry describes one real
  thing.
- *"No consumer to serve — portfolio project, no near-term release."* The
  owner reversed that; v0.1.0 is being tagged.

## Decision

A root `CHANGELOG.md`, Keep a Changelog format, semver, with an `Unreleased`
section and a `v0.1.0` entry. It covers the five tagged modules together,
since they share one version; a line says which module an entry touches when
that is not obvious.

It is **not** a second progress tracker. What it is allowed to contain is the
set a consumer cannot get anywhere else: the version, what changed in the
public API, what broke, and what will break next (the pre-v1.0 codec-layout
caveat, `ai-context/tasks/t09-codec-layout-is-unstable.md`). Everything ADR
0016 rejected it for — session-keyed narrative, re-telling `git log`,
restating `ai-context/done/` — stays out, and the two files are not in
tension: `git log` is why a line of code is the way it is, `CHANGELOG.md` is
what a caller has to change when they bump a version.

ADR 0016's four-way split of internal tracking (`git log` / `docs/adr/` /
`ai-context/done/` / `next-roadmap.md`) is unchanged. Nothing moves back out
of it.

Rejected: **GitHub release notes only.** They are the right *announcement*
surface and should carry the same text — but they live outside the module zip,
so `go get` and a vendored tree never see them, and they are invisible while
the repo is private (ADR 0019), which is exactly the window v0.1.0 ships in. A
file in the repo is readable by whoever has the code, which is the audience
that actually exists today.

Rejected: **generating the changelog from commit messages.** The entries worth
reading are the ones that say "this breaks your call site", and no commit
message in this repo was written to that spec. Hand-written, four lines a
release, is cheaper than the tooling plus the convention plus the enforcement.

Rejected: **per-module changelogs** (five files). Lockstep versioning is what
makes one file correct; five files would re-create the compatibility matrix
ADR 0019 collapsed.
