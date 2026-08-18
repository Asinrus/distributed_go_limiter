# New backend — MongoDB

**Goal:** A `mongostore` module implementing `core.CASStore` (findAndModify
with a version/state field, analogous to Redis's Lua CAS) or `core.LockStore`
(if a document-lock pattern fits better) — whichever the primitives make
simpler, Architect's call once someone scopes it.

**Independence:** Self-contained. No change to `core`, `redisstore`, or
`pgstore` — the whole point of `core.CASStore`/`core.LockStore` is that a new
backend doesn't touch existing ones. Can be picked up in any order relative
to the other vendor tasks in this folder.

**Acceptance:** `core/tck` suite passes against a real MongoDB container
(testcontainers-go, matching `redisstore`/`pgstore`'s existing pattern — see
`docs/adr/0006-testcontainers-for-backend-tck.md`). New module added to
`go.work`. `examples/mongodb/` demo mirroring the existing two.

**Status:** Unscoped — no design decisions made yet (CAS vs lock strategy,
schema shape). First step for whoever picks this up is scoping it with the
Architect, not implementing directly.
