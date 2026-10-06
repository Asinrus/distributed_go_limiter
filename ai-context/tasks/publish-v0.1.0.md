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
exist and never will. This is invisible locally because `go.work` and the
`replace` lines both resolve the modules from disk.

Live as of this writing: `redisstore/go.mod:6,65`, `pgstore/go.mod:6,67`,
`mongostore/go.mod:6,71`, `etcdstore/go.mod:5,8`, `examples/go.mod:6-10,50-55`.
`core/go.mod` has neither — it depends on nothing in this repo.

### Correction: the `replace` lines are *not* redundant

This ticket used to say they were — *"`go.work` resolves the modules from disk
… which is why the `replace` lines are redundant even today"* — and sequenced
the release to drop them first. **That was wrong**, and
`docs/adr/0022-replace-directives-are-not-redundant.md` has the measurements.
Short version: minimum-version selection needs the `go.mod` of every version
named in a `require` edge, even when the required module is a `go.work`
member supplying the packages. With the `replace` lines gone and `require` at
`v0.0.0`, `cd core && go vet ./...` and `cd redisstore && go build ./...` both
fail with `…/core@v0.0.0: module lookup disabled by GOPROXY=off`. `replace`
was the only thing satisfying that edge; `use ./core` does not.

Two consequences for this ticket, both corrections to what it claimed:

- Blocker (c) is fixed by changing the **`require`** line, not by deleting the
  `replace`. A consumer ignores our `replace` — that is the whole mechanism of
  the blocker — so `require core v0.1.0` is what they resolve, and our
  workspace keeps resolving `core` from disk with zero fetches.
- **It is one commit, not three, and no tag round in between.** The old plan's
  "this cannot be done in one commit" followed from needing the tag before the
  `require` bump could build. With `replace` kept, the bumped commit builds
  offline exactly as before, so the bump lands first and all five tags are cut
  on it.

## `examples` is not a canary — and never was

The old plan made `examples` require `core v0.1.0` and the four backends at
`v0.1.0` so that `cd examples && GOWORK=off go build ./...` would be an
in-repo reproduction of blocker (c), "failing today and passing after step 3".
It does not fail today: on a pristine tree that command passes even with
`GOPROXY=off`, because `examples` carries its own `replace` block and
`GOWORK=off` does not disable it. The check has no teeth and bumping the
versions does not give it any.

Keeping it toothless is deliberate. ADR 0022 rejects the alternative — taking
`examples` out of `go.work` and dropping its `replace` block — because
`.github/workflows/ci.yml` discovers modules with `go list -f '{{.Dir}}' -m`,
so a module outside the workspace silently stops being vetted, tested and
scanned, and because the check would then need network and private-repo
credentials on every CI run.

So `GOWORK=off go build ./...` in `examples` stays in the suite as what it
actually is: proof that `examples` compiles as a standalone module against the
on-disk siblings, outside the workspace. The reproduction of blocker (c) is
the tester's Gate 2 scratch module, outside the repo, which is where a
consumer stands. `examples` still gets no tag; nothing requires it.

## Release order

1. **One commit.** In `redisstore`, `pgstore`, `mongostore`, `etcdstore` and
   `examples`, change `require …/core v0.0.0` to `v0.1.0`; in `examples` also
   change the four store requires to `v0.1.0`. **Keep every `replace` line**,
   and put a one-line comment above each saying it is not redundant and that
   removing it breaks the offline build (point at ADR 0022). Date `t10`'s
   `CHANGELOG.md` entry — `## 0.1.0 - UNRELEASED` becomes the tag date — in
   this same commit, so the tagged tree is not self-contradictory.
   `go mod tidy` per module: expect a no-op, and note that it **cannot**
   validate the version, because `replace` short-circuits the lookup.
   `grep -rn 'distributed_go_limiter.* v0\.0\.0' */go.mod` must come back
   empty.
2. **`/verify` green, and green with `GOPROXY=off`.** This is a no-regression
   check — it proves the commit changed nothing locally, which is the point.
   It is *not* evidence about resolution; nothing in the repo can be.
3. **Merge, then tag that commit.** `core/v0.1.0` first and pushed first, so
   no store tag is ever reachable while the `core` it names is not. Then
   `redisstore/v0.1.0`, `pgstore/v0.1.0`, `mongostore/v0.1.0`,
   `etcdstore/v0.1.0`. Nested-module tags carry the directory prefix:
   `core/v0.1.0`, not `v0.1.0`. `examples` gets no tag.
4. **The consumer check (AC-61), after the tags.** The scratch module below is
   the only thing that proves blocker (c) is fixed. Run it before announcing
   the release anywhere; if it fails, the fix is a `v0.1.1` round, since tags
   do not move.

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

**Done looks like:** no repo-local `require` left at `v0.0.0`, every
`replace` still in place and commented, five `*/v0.1.0` tags, `/verify` green
under `GOPROXY=off`, and a scratch module outside the repo able to `go get`
and build against each backend.

## Test scenarios

**AC-61** — *a published module resolves and builds from outside the
workspace*: from a module that is neither in this `go.work` nor supplying a
`replace` for it, each of the five modules at `v0.1.0` resolves its `core`
dependency to a real version and compiles. The qualifier matters: "outside
the workspace" means *outside the repo*, because in-repo every store's own
`replace` satisfies the edge from disk (ADR 0022). Verified free by grepping
`AC-[0-9]+` across the full history of every ref, not just `main`; the highest
claimed anywhere is AC-60 (`t06`).

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
- `cd examples && GOWORK=off go build ./...` inside the repo: green before and
  after, including with `GOPROXY=off`. It proves `examples` compiles as a
  standalone module against the on-disk siblings — **not** that any version
  resolves. Do not report it as evidence for AC-61; see "examples is not a
  canary" above for why it cannot be.
- `e2e/k6/run-sandbox.sh redis` once as a no-regression pass (5 accepted, 3
  rejected; refill checks pass). Nothing in this ticket changes behaviour, so
  a difference here means a dropped dependency, not a limiter change.

**Unit / integration (Dev):** no new tests. `/verify` green after step 1,
**and green with `GOPROXY=off`**, is the assertion — it proves the commit is
inert locally, which is what keeping the `replace` lines buys. Do not try to
confirm "the tagged `core` is the one being compiled" from inside the repo: it
never is, and a `GOWORK=off` run in a backend resolves `core` through that
backend's `replace` to the working tree (ADR 0022). Compiling against the
tagged `core` is the Gate 2 scratch module's job.
