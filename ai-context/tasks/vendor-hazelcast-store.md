# New backend — Hazelcast

**Goal:** A `hazelcaststore` module implementing `core.CASStore` — Hazelcast's
`Map.ReplaceIfSame`/optimistic-locking primitives are the natural fit for
compare-and-swap; confirm during scoping.

**Independence:** Self-contained. No change to `core`, `redisstore`, or
`pgstore`. Can be picked up in any order relative to the other vendor tasks
in this folder.

**Acceptance:** `core/tck` suite passes against a real Hazelcast container
(testcontainers-go, matching the existing pattern — see
`docs/adr/0006-testcontainers-for-backend-tck.md`). New module added to
`go.work`. `examples/hazelcast/` demo mirroring the existing two.

**Status:** Unscoped — first step for whoever picks this up is scoping it
with the Architect, not implementing directly.

**Note:** Memcached was considered alongside this and dropped from scope —
see `ai-context/PLANS.md` changelog.
