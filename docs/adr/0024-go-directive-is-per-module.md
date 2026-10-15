# The `go` directive is per module, forced by that module's own dependencies

Supersedes `docs/adr/0017-one-go-directive-tracks-the-dependency-floor.md`.

Each `go.mod` states the floor **its own** dependency graph forces, and nothing
higher. `go.work` states the highest floor among its members, because the
workspace has to build all of them.

```
core        go 1.25.0   zero third-party runtime deps; forces nothing
redisstore  go 1.25.0
pgstore     go 1.25.0
mongostore  go 1.25.0
etcdstore   go 1.26.0   forced by golang.org/x/net v0.60.0
examples    go 1.26.0   same, and never tagged (docs/adr/0019)
go.work     go 1.26.0   the highest member
```

0017's title clause is what falls: one number in every `go.mod` and `go.work`,
no per-module exception. What survives 0017 is carried into this file rather
than left behind a supersession pointer — no `toolchain` directive, the floor
is derived and not chosen so a bump always has a named cause, chasing the
newest Go release stays rejected, and 0017's measurement method, which is now
the whole mechanism.

The CI toolchain pin is untouched.
`docs/adr/0023-ci-toolchain-pin-tracks-a-supported-release-line.md` owns it and
nothing here changes it; the two knobs 0023 named stay named. 0023 says in
passing that 0017's ruling "is not reopened", which was true when it was
written and is not any more — it bears only on the paragraph of 0017 that 0023
replaced, not on 0023's own ruling.

## What forced it

Pinning CI to `1.26.9` closed seven stdlib advisories and uncovered the same
bugs a second time, in `golang.org/x/net`'s own HTTP/2 implementation.
GO-2026-6617, -6612, -6611 and -6603 each list `stdlib` fixed in `1.26.9`
**and** `golang.org/x/net` fixed in `v0.60.0`. A toolchain pin closes the first
copy and can do nothing about the second, which is why the pin appeared to
reveal new advisories rather than clearing the slate.

`x/net` is indirect here: `etcdstore` carries it via
`go.etcd.io/etcd/client/v3` and gRPC, `examples` via the etcd client and
`testcontainers-go`. `core` and `redisstore` do not require it at all.

`golang.org/x/net v0.60.0` declares `go 1.26.0`. That is the entire forcing
fact.

GO-2026-6629 (`x/text/secure/precis`, fixed in `x/text v0.41.0`) rides along
and forces nothing: `v0.41.0` declares `go 1.25.0`. Worth naming anyway,
because `pgstore` and `mongostore` do require `x/text` indirectly (`v0.29.0`
and `v0.17.0`), so the fix is available to them without moving their floor —
but only if the bump targets the **fixed** version. `x/text v0.42.0`, the one
`x/net v0.60.0` pulls in, declares `go 1.26.0`; taking the newest instead of
the fixed version would raise two more modules for nothing.

## What was rejected

**One number everywhere at `1.26.0`.** This honours 0017 literally and is the
option 0017's text points at. Its cost: every consumer of `core` — a module
with zero third-party runtime dependencies — would need Go 1.26 because of a
vulnerability in a transitive dependency of the etcd client, and users on
Go 1.25, a line the Go project still supports, could import none of it.

0017 accepted exactly that cost once, on a ground that has since gone: that
`core`'s advertised floor was unavailable anyway, because the flagship
provider-swap path imports `pgstore` and `pgstore`'s graph already required
`1.25.0`, so a lower number in `core` protected only the in-memory reference
path. Under this ruling `core`, `redisstore`, `pgstore` and `mongostore` are
all at `1.25.0`. The provider-swap path `CLAUDE.md` advertises as the primary
design goal holds the floor end to end, and the single module above it is the
one whose vendor client drags in gRPC. 0017's argument now cuts the other way.

**Ship v0.1.0 unfixed.** Four advisories with reachable symbols in `etcdstore`
and a published fix for all four. Not a trade-off.

## Why this is not 0014's escape hatch returning

0017 killed a per-module hatch and gave reasons, so the burden is here.

- **This is the rule, not an exception to one.** 0014 ruled one number and then
  let the Architect raise individual modules; the hatch fired three times in
  days and left three numbers that had to be reconciled against a single number
  nobody could point at. There is nothing to reconcile here. "Why is this module
  `1.26.0`?" has one answer, in that module's own `go.mod`, and
  `grep '^go ' go.work */go.mod` is the complete picture. This is Go's own
  model — a module declares what its dependencies require — and 0017's one
  number was the deviation from it.
- **This is not discretion.** No agent raises a module because another module
  moved, because CI runs something newer, or because a number looks tidy. The
  only thing that raises a `go.mod` is that module's own graph. A raise with no
  dependency version named behind it is a violation of this rule, not an
  exercise of it.
- **0021 removed the premise 0017 leaned on.** 0017's strongest practical
  complaint was that nobody could answer *"what version is this repo?"* from
  the tree. After `docs/adr/0021-modules-version-independently-after-v0-1-0.md`
  that is not a question a consumer asks: the modules carry independent
  versions, are fetched independently and are consumed independently. The
  questions are "what does `core` require?" and "what does `etcdstore`
  require?", and each has a one-file answer.
- **0023 already separated the other surviving justification.** *The version
  local runs and CI agree on is the version a failure reproduces at* is 0014's
  line, inherited by 0017. It is about the CI pin, and 0023 made the pin an
  independent knob — today `1.26.9`, above every directive in the tree.
  Nothing is tested at `1.25.0` and nothing was before; the directive states a
  floor a consumer's toolchain must clear and never named the reproduction
  version. A spread of directives costs that justification nothing.
- **0017's own named revisit points here.** 0017 said that if this repo
  acquired real consumers the fix would be "a supported-version policy with a
  test matrix, not a directive that quietly disagrees with the module graph."
  A `1.26.0` in `core/go.mod` is precisely a directive that disagrees with
  `core`'s module graph.

`examples` concedes nothing at all: it is a pair of `main` packages that is
never tagged (`docs/adr/0019-v0-1-0-is-five-lockstep-modules-under-apache-2-0.md`),
so there is no consumer on the other side of its number. Of the seven files,
exactly one published module moves.

## What this costs

Two numbers in the tree instead of one, and a reader who wants a repo-wide
answer reads six files rather than one. Accepted, because each of the six is
authoritative for itself, which the single number never was.

A consumer who imports `etcdstore` needs Go 1.26, unavoidably, while the fix
lives in a module that declares it.

The spread can widen, and a `go.work` whose number is the maximum hides members
whose own floor is lower — which is the hazard 0017 named, and the reason the
measurement rule below is not optional.

## Measuring the floor

Unchanged from 0017, and load-bearing now rather than incidental. Per module,
with `GOWORK=off`, from that module's own `go.mod` requirements: the workspace
unifies transitive versions across members and hides each module's real floor.
Read the dependency's own `.mod` file — `proxy.golang.org/<module>/@v/<version>.mod`
answers it in one request. Do **not** run `go mod tidy` to find out; inside
this workspace it rewrites the `go` directive upward instead of reporting a
conflict, answering the question by destroying the evidence.

## When to revisit

- A module's directive moves with no dependency version named as the cause.
  That is 0014's hatch, and 0017's reasoning against it applies in full.
- The spread stops being recomputable: distinct numbers accumulating faster
  than a reader can trace each one to a `require` line.
- `core`'s own floor is forced up by its own dependencies. There is nothing to
  decide then — `core` has no third-party runtime dependencies, so such a raise
  would have to come from somewhere this rule does not recognise.
- This repo acquires consumers on a line it no longer clears. That is 0017's
  revisit, unchanged: a supported-version policy with a test matrix.

Deliberately not added: any process for tracking Go releases or dependency
updates, and no vulnerability-exception mechanism. `govulncheck` in CI is the
forcing function that produced this ADR and is the one that will produce the
next bump.
