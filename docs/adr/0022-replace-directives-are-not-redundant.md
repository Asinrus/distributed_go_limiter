# The `replace` lines stay; the `require` line carries the released version

`ai-context/tasks/publish-v0.1.0.md` sequenced the release as: drop every
`replace github.com/asinrus/distributed_go_limiter/core => ../core`, tag
`core/v0.1.0`, then bump each store's `require` to it. Its stated reason was
that *"`go.work` resolves the modules from disk — which is exactly what
`go.work` is for, and why the `replace` lines are redundant even today"*.

**That reason is false, and the ruling is the opposite of it.** The `replace`
lines are load-bearing and permanent. The release is one commit that changes
each `require` to `core v0.1.0` and keeps every `replace`, followed by five
tags on that commit. There is no intermediate state in which the repo cannot
build, because no intermediate state is needed.

## Measured, not reasoned

With the `replace` lines removed and the `require` lines left at `v0.0.0`
(Go 1.25.6, `GOPROXY=off`, `go.work` unchanged):

```
cd core && go build ./...   # clean
cd core && go vet ./...
  available_tokens_negative_test.go:8:2: …/core@v0.0.0: module lookup disabled by GOPROXY=off
cd redisstore && go build ./...
  redisstore.go:14:2: …/core@v0.0.0: module lookup disabled by GOPROXY=off
```

`go build` in `core` passes because `core` requires nothing in this repo;
everything that has to resolve the `require` edge fails. The breakage is wider
than a test-only problem — a plain `go build` in each store fails too.

Changing the `require` to `v0.1.0` does not help while no such tag exists, and
the error proves the mechanism: the lookup is attempted, so `use ./core` is
**not** satisfying the requirement edge. Minimum-version selection needs the
`go.mod` of every version named in a `require` edge to compute the build list,
even when the required module is a workspace member that supplies the packages.
(It does not fire in a toy workspace — a two-module repro with a
dependency-free library passed. It fires once the members' dependency sets
differ, as all six here do.)

What the fetch is *for* matters, and it is the one thing that argued for the
other option: with a file-based proxy serving a synthetic `core@v0.1.0`, the
build succeeds and `go list -deps` still reports the `core` package `Dir` as
the on-disk `core/`. Only the `go.mod` comes from the tag. So dropping
`replace` would not change what gets compiled — it would add a mandatory fetch
of this repo's own tags, from a **private** host, to `cd core && go test`.

Rejected: **tag `core/v0.1.0` first, then drop `replace` and bump `require` in
one commit.** It removes the broken intermediate and it is the only variant
that could give the in-repo `examples` canary teeth. It also makes every clone
need network and credentials for this repo's own tags before anything builds —
permanently, for a repo that is private. A tag cannot be withdrawn once
fetched, so this is not a property to discover after tagging.

## What this gives up, said plainly

Nothing in the repo now validates that the version in a `require` line exists.
`go mod tidy` is a no-op on it — measured: with `require core v0.1.0` and the
`replace` in place, `go mod tidy` left `redisstore/go.mod` byte-identical and
wrote no `go.sum` entry for `core`. A typo in that version ships.

The check that catches it is a module outside this repo, which is where a
consumer is. `cd examples && GOWORK=off go build ./...` is not that check: it
passes today, under `GOPROXY=off`, because `examples` carries its own
`replace` block. It never had teeth, so this ruling takes none away — it only
stops the ticket claiming otherwise.

Rejected: **taking `examples` out of `go.work` and dropping its `replace`
block**, which would turn that command into a real consumer check inside the
repo. Two costs killed it. `.github/workflows/ci.yml` discovers the module
list with `go list -f '{{.Dir}}' -m`, so a module outside `go.work` silently
stops being vetted, tested and vulnerability-scanned — a check bought by
losing four others. And the check itself would need network and private-repo
credentials on every CI run, which is the dependency this ruling exists to
avoid.

Rejected: **dropping the `require` line instead of the `replace`.** A released
module must name the `core` it was built and tested against — that single line
is ADR 0021's answer to "which `core`?" — and `replace` alone does not put a
module in the build list.

## Consequences

Each `replace` gets a comment saying it is not redundant and what breaks if it
is tidied away. A line that three separate readings called dead weight needs
the reason next to it, not in a ticket that moves to `ai-context/done/`.

`docs/adr/0021-modules-version-independently-after-v0-1-0.md` is amended: its
"any pair is checkable" oracle, `cd <store> && GOWORK=off go test ./...`,
resolves `core` through the store's `replace` to the working tree, not to the
version in `require`. Verified: `GOWORK=off go list -m .../core` in
`redisstore` reports `"Version": "v0.1.0"` with `"Replace"` pointing at
`../core`. The command is right for developing a pair and wrong for certifying
a released one; 0021 now says which is which.

**Correction, 2026-10-20.** The rejected alternative above prices a mandatory
fetch of this repo's own tags as a cost paid "permanently, for a repo that is
private". The repo went public on 2026-10-20, so that fetch would no longer
need credentials and the clause has expired. The ruling is unchanged: the
`replace` lines stay, because making `cd core && go test` depend on a network
fetch of this repo's own tags is worth avoiding whether or not the fetch is
authenticated.
