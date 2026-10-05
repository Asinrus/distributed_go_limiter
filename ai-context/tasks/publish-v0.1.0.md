# Publish v0.1.0 — tag five modules, fix the `replace` trap

**Active.** The "portfolio / reference implementation, no near-term release"
ruling that kept this file dormant is **reversed** — the owner wants v0.1.0
prepared. The four decisions that came with the reversal are ratified in
`docs/adr/0019-v0-1-0-is-five-lockstep-modules-under-apache-2-0.md`:

1. **The repo stays private.** Tag everything, publish nothing.
2. **Apache-2.0**, in the repo root and all six module roots — `t07`.
3. **PRs #25–#30 land first.** v0.1.0 ships with AC-53/AC-54, `ProxyOption`
   forwarding, and the widened conformance suite. All six are merged as of
   `890f6b1`, so this precondition is met.
4. **All five modules start at `v0.1.0`** — `core`, `redisstore`, `pgstore`,
   `mongostore`, `etcdstore`. `examples` is never tagged. 0019's "and move
   together from here" is **reversed** by
   `docs/adr/0021-modules-version-independently-after-v0-1-0.md`: after this
   release each module is tagged on its own, when its own code changes. That
   changes nothing in the steps below — the first release still puts the same
   number on all five — but it is why no step here promises future lockstep.

**For:** Dev, then the tester. This file is the **tagging** ticket: blocker (c)
and the tag sequence. The release's other work has its own tickets — `t07`
(license), `t08` (README/doc accuracy), `t09` (codec-layout promise), `t10`
(CHANGELOG), `t11` (pkg.go.dev first impression). This one goes **last**: the
`v0.1.0` tags should point at a commit where all of those have landed, because
a tag cannot be moved after anyone fetches it.

**Goal:** `go get github.com/asinrus/distributed_go_limiter/redisstore` works
for someone who has never seen this repo, and resolves `core` at a real
version. While the repo is private that "someone" is the owner with
`GOPRIVATE` set; after visibility flips it is anyone, and pkg.go.dev indexes.

## Blocker (a) — private: deferred, not resolved

pkg.go.dev only indexes public modules, so **nothing will appear there** until
visibility flips. v0.1.0 is therefore *tagged but unpublished*: the artefacts
exist and are verifiable by the owner, the public documentation surface does
not exist yet. Flipping visibility later is a settings change and needs no
code, which is why the rest of the release does not wait on it. Reasoning in
ADR 0019.

## Blocker (b) — license: `t07`

`ai-context/tasks/t07-apache-2-0-license.md`. Must land before the tags —
pkg.go.dev renders no documentation for a module whose license it cannot
recognise, and a tag without one is a tag that has to be superseded.

## Blocker (c) — the `replace` trap, the actual latent breakage

Every backend `go.mod` has both:

```
require github.com/asinrus/distributed_go_limiter/core v0.0.0
replace github.com/asinrus/distributed_go_limiter/core => ../core
```

A `replace` directive in a *dependency's* `go.mod` is ignored by the consuming
build; only the main module's `replace` applies. So the first outside
`go get .../redisstore` fails trying to resolve `core v0.0.0`, which does not
exist and never will. This is invisible locally because `go.work` resolves the
modules from disk — which is exactly what `go.work` is for, and why the
`replace` lines are redundant even today.

Live as of this writing: `redisstore/go.mod:6,65`, `pgstore/go.mod:6,67`,
`mongostore/go.mod:6,71`, `etcdstore/go.mod:5,8`, `examples/go.mod:6-10,50-55`.
`core/go.mod` has neither — it depends on nothing in this repo.

**This cannot be done in one commit, and that is not a mistake in the plan.**
Step 3 requires a version that only step 2 creates. Whoever executes should
expect three commits and one tag round in between, and should not try to
collapse them: `require core v0.1.0` pushed before `core/v0.1.0` exists leaves
`main` unbuildable for anyone outside the workspace, which is the state this
whole ticket exists to end.

## Release order

1. Drop the `replace` lines from `redisstore`, `pgstore`, `mongostore`,
   `etcdstore`, and `examples`. Local development keeps working through
   `go.work`. **`/verify` must still be green after this commit** — that is
   the proof `go.work` alone carries local development.
2. Tag `core/v0.1.0` first — nothing else can require a real version until it
   exists. Nested-module tags carry the directory prefix: `core/v0.1.0`, not
   `v0.1.0`.
3. Change each backend's `require` to `core v0.1.0`. **Including `examples`**
   — see "examples is the canary" below. `go mod tidy` per module; the
   resulting `go.sum` entries are the first real check that the tag resolves.
4. Tag `redisstore/v0.1.0`, `pgstore/v0.1.0`, `mongostore/v0.1.0`,
   `etcdstore/v0.1.0`. `examples` gets no tag.
5. `t10`'s `CHANGELOG.md` gets its `v0.1.0` entry dated at this point, not
   before.

## `examples` is the canary

The original plan dropped `examples`' `replace` block but left it requiring
`v0.0.0`. Requiring `core v0.1.0` and the four backends at `v0.1.0` instead
makes `examples` an exact model of what an outside consumer writes — so
`cd examples && GOWORK=off go build ./...` becomes an **in-repo** reproduction
of blocker (c), runnable in CI, instead of a check that only exists in a
scratch directory someone has to remember to create. It fails today and must
pass after step 3. `examples` still gets no tag; nothing requires it.

## Module path casing — verify, do not assume, and expect no edit

The GitHub repo is `Asinrus/distributed_go_limiter` (capital A); every
`go.mod` declares `github.com/asinrus/...` (lowercase). Go module paths are
case-sensitive and the proxy escapes uppercase as `!a`, so the mismatch is
worth a look — but the expected outcome is that it is **fine and preferable**:
GitHub resolves owner names case-insensitively, the declared path and the
`require` path agree with each other (which is what Go's consistency check
compares), and a lowercase path avoids `!`-escaped proxy URLs. Confirm it in
the Gate 2 fetch rather than assuming either way. **No `go.mod` edit is
expected here**; if the fetch does fail on casing, stop and raise it — a
module path change is a different, much larger ticket.

## Out of scope

- Flipping the repo to public, and the commit-history cleanup the owner wants
  before that. Separate, owner-driven.
- Release automation and CI workflow changes (`.github/workflows/` is not this
  ticket's).
- What a version means beyond this first tag — ruled in ADR 0021 (independent
  per-module versioning and the `core`-interface compatibility contract) and
  ADR 0020 (changelog), not re-opened here.
- Any `v0.2.0` planning. Row 17.

**Done looks like:** no `replace` of a repo-local module anywhere, five
`*/v0.1.0` tags, `cd examples && GOWORK=off go build ./...` green, and a
scratch module outside the repo able to `go get` and build against each
backend.

## Test scenarios

**AC-61** — *a published module resolves and builds from outside the
workspace*: with `GOWORK=off` and no `replace` directive supplying it, each of
the five modules at `v0.1.0` resolves its `core` dependency to a real version
and compiles. Verified free by grepping `AC-[0-9]+` across the full history of
every ref, not just `main`; the highest claimed anywhere is AC-60 (`t06`).

**Black-box (tester, Gate 2):**
- In a scratch directory outside the workspace: `go mod init scratch`, then
  `go get` each of the five modules at `v0.1.0`, and build a program that
  constructs a `ProxyManager` from one of them and calls `TryConsume`. Must
  resolve and compile with `GOWORK=off`. While the repo is private this needs
  `GOPRIVATE=github.com/asinrus/*` and git credentials with read access —
  that is the authenticated-owner form of the check, and it exercises the same
  resolution path a stranger will.
  **This is the only check that catches blocker (c)** — every in-repo gate
  passes today precisely because `go.work` hides it.
  `GOPRIVATE` bypasses the public proxy and the checksum database for these
  paths, so this fetch goes straight to GitHub; `go.sum` entries are still
  written and still verified locally against the fetched tag, which is the
  part that must agree.
- `cd examples && GOWORK=off go build ./...` inside the repo: must fail before
  step 3 and pass after. Confirming the *failure* first is what proves the
  check has teeth.
- `e2e/k6/run-sandbox.sh redis` once as a no-regression pass (5 accepted, 3
  rejected; refill checks pass). Nothing in this ticket changes behaviour, so
  a difference here means a dropped dependency, not a limiter change.

**Unit / integration (Dev):** no new tests. `/verify` green after step 1 is the
assertion — it proves the `replace` lines were dead weight. After step 3,
`go test ./...` per module must still pass with the real `core` version
resolved from the tag rather than from disk; run at least one backend's suite
with `GOWORK=off` to confirm the tagged `core` is the one being compiled.
