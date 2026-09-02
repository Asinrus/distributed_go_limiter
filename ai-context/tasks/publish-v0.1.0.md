# Publish v0.1.0

**Dormant — do not pick this up.** The owner has ruled this a portfolio /
reference implementation with no near-term public release. The file stays
because blocker (c) below is a real latent defect that `go.work` hides
completely: every in-repo gate passes today and the first outside `go get`
would still fail. Recording it now costs nothing; rediscovering it on release
day costs a day. Revisit only if the answer to "are we publishing" changes.

**Goal (when it is picked up):** `go get
github.com/asinrus/distributed_go_limiter/redisstore` works for someone who
has never seen this repo, and pkg.go.dev shows the docs.

Three blockers. Two are the owner's call, one is a latent defect in the repo.

**(a) The repository is private.** `Asinrus/distributed_go_limiter` is not
public, and pkg.go.dev only indexes public modules. **Owner's decision, not the
Architect's** — nothing else here matters until it is made.

**(b) There is no LICENSE.** No `LICENSE` file exists at the root or in any
module. pkg.go.dev renders no documentation for a module without a license it
recognizes, so this is not only a legal question. For a multi-module repo the
file goes in **each module root**, since each module is fetched on its own.
Which license (MIT / Apache-2.0 / BSD-3-Clause) is the **owner's decision**.

**(c) The `replace` trap — the actual latent breakage.** Every backend
`go.mod` has both:

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

**Release order** (each step must be pushed before the next resolves):
1. Drop the `replace` lines from `redisstore`, `pgstore`, `mongostore`,
   `etcdstore`, and `examples`. Local development keeps working through
   `go.work`.
2. Tag `core/v0.1.0` first — nothing else can require a real version until it
   exists.
3. Change each backend's `require` to `core v0.1.0`.
4. Tag `redisstore/v0.1.0`, `pgstore/v0.1.0`, `mongostore/v0.1.0`,
   `etcdstore/v0.1.0`. `examples` is not published and needs no tag.

**Out of scope**
- Release automation, changelog generation, and versioning policy beyond this
  first tag. What a version *means* across five independently tagged modules is
  an open question in `ai-context/tasks/plans-md-vs-changelog.md`; this task
  tags 0.1.0 and does not settle it.

**Done looks like:** a fresh module outside this repo can
`go get` each backend and build against it, and pkg.go.dev lists all five.

## Test scenarios

**Black-box (tester, Gate 2):** in a scratch directory outside the workspace,
`go mod init`, `go get` each of the five published modules, and build a program
that constructs a `ProxyManager` from one of them. It must resolve and compile
with `GOWORK=off`. This is the only check that catches blocker (c) — every
in-repo gate passes today precisely because `go.work` hides it.

**Unit / integration (Dev):** none new. After step 1, `/verify` must still be
green in the workspace, proving `go.work` alone carries local development once
the `replace` lines are gone.
