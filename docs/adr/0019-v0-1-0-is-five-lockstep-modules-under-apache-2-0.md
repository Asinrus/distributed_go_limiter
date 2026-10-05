# v0.1.0 is five lockstep-tagged modules under Apache-2.0, tagged but unpublished

The owner reversed the "portfolio / reference implementation, no near-term
release" ruling that kept `ai-context/tasks/publish-v0.1.0.md` dormant and
that ADR 0016 leaned on. Everything for a first release is to be prepared.
Four decisions came with the reversal; they interlock, so they are recorded
together rather than as four files.

## Apache-2.0, in all seven roots

`LICENSE` is Apache-2.0, copied verbatim into the repo root **and each of the
six module roots** (`core`, `redisstore`, `pgstore`, `mongostore`,
`etcdstore`, `examples`). Seven copies is not redundancy: a Go module is
fetched on its own, so the only `LICENSE` a consumer of `redisstore` ever
receives is the one inside `redisstore/`, and pkg.go.dev renders no
documentation for a module whose license it cannot recognise. `examples` is
included even though it is never tagged — a reader who copies from it should
see the terms they are copying under.

Rejected: **MIT**, which is shorter and more common in Go. Apache-2.0 carries
an express patent grant, and it is bucket4j's own license — this library
carries bucket4j's semantics across (`docs/adr/0018-deliberate-deviations-from-bucket4j.md`),
and matching the upstream license keeps the lineage free of a license question
nobody wants to litigate. The cost is a 200-line file in seven places instead
of a 20-line one.

Rejected: **one `LICENSE` at the root only.** It is what a human browsing the
repo sees and nothing else — not the module zip, not pkg.go.dev.

## One `NOTICE`, at the repo root, and a lineage sentence in `core`'s package doc

bucket4j ships **no** `NOTICE` file (checked against the upstream checkout:
`LICENSE.txt`, Apache-2.0, and no `NOTICE`), so Apache-2.0 §4(d) — which
obliges a derivative work to carry the upstream's notice text — has nothing to
propagate here. A `NOTICE` in this repo is therefore voluntary attribution,
not compliance.

It is still worth writing, once, because the lineage claim is the first
question any reader of a port asks ("is this a copy?") and it currently lives
only in `README.md`'s opening sentence and in ADR 0018 — one marketing line
and one process document, both of which a docs refresh can reword. `NOTICE`
states it flatly: no bucket4j source was copied, the semantics were
reimplemented, bucket4j is Apache-2.0 and copyright its authors.

Rejected: **a `NOTICE` in all seven roots.** No tooling consumes it, §4(d)
does not bind, and seven copies of a prose statement is seven chances to
drift. The module-fetch audience is served instead by one sentence in `core`'s
package doc — which is the page pkg.go.dev shows and which has to be edited
anyway, since it still names only "local, Redis, Postgres".

Rejected: **no `NOTICE` at all.** Defensible, since the README says it — but
the README is the file most likely to be rewritten by someone who does not
know the sentence was load-bearing.

## Lockstep versioning: one number across five modules

`core`, `redisstore`, `pgstore`, `mongostore` and `etcdstore` are all tagged
`v0.1.0`, and move together from here. `examples` is never tagged — it is a
demonstration, not a product, and tagging it would publish four `main`
packages as a library.

This is deliberately *not* the Go default of independent per-module versions.
The trade-off: independent versions are honest about what changed, but they
produce a compatibility matrix (does `redisstore v0.3.1` work with
`core v0.2.4`?) that nobody can answer without reading five `go.mod` files —
and every backend's only non-test dependency on this repo is `core`, so the
matrix has exactly one interesting axis anyway. Lockstep collapses it: the
same number everywhere means the combination was tested together. The cost is
no-op bumps — a `core`-only fix re-tags four backends whose code did not
change. That is accepted; a tag is cheap and a matrix is not.

This also gives ADR 0016's rejected alternative (2) the thing it lacked: a
version with a stated scope. "0.1.0" means *this repo's five library modules
at this commit*, which is why `docs/adr/0020-changelog-starts-at-v0-1-0.md`
can now exist.

Rejected: **independent per-module versions** (the Go convention) — the matrix
above. Rejected: **a single root module** instead of five, which would remove
the question entirely but also remove the property the library is built to
demonstrate: that a consumer takes `core` plus the one backend it uses and no
vendor dependency it does not.

## Tagged, not published — blocker (a) is deferred, not resolved

The repository stays private for now ("keep private; we need to finish all +
clean a commit history a little bit"). The tags are cut; nothing is announced.

The consequence to state plainly, because it is easy to assume otherwise:
**pkg.go.dev will not index anything while the repo is private**, and
`go get` works only for a caller who is authenticated against the repo
(`GOPRIVATE=github.com/asinrus/*`). So the v0.1.0 release is *tagged but
unpublished*: the artefacts exist and are verifiable by the owner, and the
documentation surface a stranger would see does not exist yet. Blocker (a) in
`ai-context/tasks/publish-v0.1.0.md` is deferred to a second, trivial step —
flip visibility, no code change — not resolved by this round.

Why cut the tags at all before going public: the latent defect the release
exists to fix (`replace` + `require core v0.0.0`, blocker (c)) **cannot be
fixed without a real version to require**, and cannot be verified without a
fetchable tag. Waiting until the repo is public means discovering it on the
day it is most expensive.
