# New backend — etcd

**Goal:** An `etcdstore` module implementing `core.CASStore` — etcd's native
`Txn` with a `ModRevision` compare is a close match to the CAS contract
(closer than a lock-based fit), so `CASStore` is the likely choice, not
`LockStore`; confirm during scoping.

**Independence:** Self-contained. No change to `core`, `redisstore`, or
`pgstore`. Can be picked up in any order relative to the other vendor tasks
in this folder.

**Acceptance:** `core/tck` suite passes against a real etcd container
(testcontainers-go, matching the existing pattern — see
`docs/adr/0006-testcontainers-for-backend-tck.md`). New module added to
`go.work`. `examples/etcd/` demo mirroring the existing two.

**Status:** Unscoped — first step for whoever picks this up is scoping it
with the Architect, not implementing directly.
