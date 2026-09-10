# Question — should etcd stay a main backend?

**This is a question for the repo owner, not a work item.** It carries no AC id
and nothing is implemented from it; the shape follows
`ai-context/tasks/vendor-hazelcast-store.md`. The Architect's job here is to
lay out the trade-off and the options, not to pick one.

**Origin:** the owner, on deferring AC-52 — *"It should be the separate task to
discuss the existence of etcd as a main backend. I think we could discuss
later."*

## Why it is being asked now

The vendor ctx-conformance guard (AC-52, roadmap row 20 — its task file lands
with PR #19 and is not on this branch) is deferred on this. Every conformance case the project adds is written once and
paid for four times, so how many backends the project carries changes what that
work is worth. The same multiplier applies to roadmap row 8 (`ProxyOption`
forwarding) and to anything row 17's codec change touches. This is the moment
the fourth backend starts charging rent.

## What four backends cost, specifically

Facts checked against `origin/main`, not estimated:

- **etcd is the only store module that pulls gRPC.** `google.golang.org/grpc
  v1.64.1` appears in `etcdstore/go.mod` and, transitively, `examples/go.mod` —
  nowhere else. Open PR #18 (a grpc `GO-2026-6061` vulnerability bump, itself
  blocked on the Go 1.23 pin) exists because of this module. That is a
  recurring, externally-timed cost the other three do not impose.
- **Dependency weight:** 65 `require` lines in `etcdstore/go.mod` against 54
  (redis), 56 (pgstore), 60 (mongo). The gap is gRPC, protobuf, genproto and
  zap. Modest on its own; it is the *maintenance* of that subtree, not its
  size, that shows up as PR #18.
- **A second container registry in CI.** `etcdstore/etcdstore_test.go:42` pulls
  `quay.io/coreos/etcd:v3.5.16`; the other three pull `redis:7-alpine`,
  `postgres:16-alpine` and `mongo:7` from Docker Hub.
- **The slowest test in the suite.** `etcdstore/etcdstore_test.go:132` sleeps 3
  real seconds, because etcd's server-side minimum lease TTL is roughly three
  times its election timeout (`docs/client/etcd.md:26-30`). Roadmap row 8 would
  make that injectable for the *other* backends; etcd's floor is the server's,
  not the client's.
- **An open correctness defect of its own.** Roadmap row 4 — a lease granted
  per `Txn` attempt including lost CAS races, never revoked
  (`etcdstore/etcdstore.go:39,75`), roughly 360k live leases at 100 rps with
  the example's 1h TTL, and 2 RPCs per token check. PR #15 fixes it; no other
  backend has needed an equivalent.

## What etcd earns

- **It is a `CASStore`, and so are Redis and Mongo.** The provider-swap claim
  rests on the `CASStore`/`LockStore` split, and that split is already
  demonstrated by two backends. etcd is the third instance of the primitive the
  project already proves — which is real evidence for pluggability, just with
  diminishing returns per backend.
- **Its CAS is the most literal of the three.** `Txn` comparing on
  `clientv3.Value` is compare-and-swap as the interface describes it, with no
  Lua script and no conditional-replace idiom in between. That makes it the
  clearest worked example for someone writing a fifth backend.
- **Portfolio breadth.** By owner ruling (1) this repo is a portfolio /
  reference implementation, not a near-term release. Under that goal "four
  stores, one suite" reads differently than it would for a shipping library,
  where each backend is a support obligation.
- **It works today.** TCK green on a real container, k6 sandbox green
  (`ai-context/PLANS.md` phase 8). Nothing here is a rescue; the question is
  whether to keep paying, not whether to finish.

## The options

1. **Keep it as a main backend.** Row 20's conformance work covers four; row 8
   plumbs four; PR #18 and its successors get worked as they arrive.
2. **Keep it, demote it to "contributed".** It stays in the workspace and keeps
   passing the TCK, but new cross-cutting work (row 20, row 8, row 17) is
   defined as satisfied by Redis, Postgres and Mongo, and etcd follows when
   convenient. Cheapest option that deletes nothing; the cost is that "every
   backend runs the same suite" stops being literally true, which is the exact
   claim `README.md` makes.
3. **Move it out of the workspace** — its own repo or a `contrib/` module
   outside `go.work`. Removes the gRPC subtree and the second registry from CI
   entirely, and makes it the first real test of whether a backend can be built
   *against* this project from outside, which is the pluggability claim's
   strongest possible evidence. Costs: a second repo to keep alive, and the
   `replace` directive problem in `ai-context/tasks/publish-v0.1.0.md` becomes
   blocking rather than dormant.
4. **Drop it.** Deletes `etcdstore/`, `examples/etcd/`, `docs/client/etcd.md`,
   roadmap row 4 and PR #15. Loses a working, tested backend to save
   maintenance — the least reversible option, and the only one that throws away
   finished work.

## What the answer unblocks

- AC-52, the vendor ctx-conformance guard (roadmap row 20, PR #19) — deferred
  explicitly on this.
- Roadmap row 8's scope (four backends or three).
- Whether row 4 / PR #15 is worth merging.

**Status:** open, deferred by the owner ("we could discuss later"). No work
starts from this file until it is answered; whoever answers it records the
reason as an ADR if the choice is options 2-4, since all three are hard to
reverse and cut against `README.md`'s current claim.

---

## Answer (owner ruling): etcd stays, as a peer

**There are no backend tiers.** The options above were framed as a ranking —
"main" versus "contributed" versus out-of-workspace — and that framing is
**withdrawn**. The local path is the main one; every store under
`ProxyManager` is a peer of every other. Options 2, 3 and 4 all presuppose a
tier system that does not exist, so none of them is on the table.

Go modules are independent, so a backend nobody imports costs a consumer
nothing: `etcdstore/`'s gRPC subtree never enters the build of someone using
`redisstore`. The cost is therefore CI upkeep only — and CI upkeep is not a
reason to delete working, tested code. A `govulncheck` failure in any module
fails the whole workspace, which is the cost being accepted here, deliberately
and with eyes open.

**What this unblocks:** roadmap row 20 (vendor ctx conformance) is un-deferred
and covers four backends; roadmap row 8 (`ProxyOption` forwarding) plumbs four.

**One citation above is stale.** The 3s sleep is at
`etcdstore/etcdstore_test.go:245` today, not `:132`; `:132` is now the
lease-count test, which has no sleep. The underlying point stands — etcd's
minimum lease TTL is server-side and `core.WithClock` cannot shorten it, so
roadmap row 8 does not make that test instant. See `t04-proxyoption-backends.md`.
