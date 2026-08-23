# New backend — MongoDB

**Goal:** A `mongostore` module implementing `core.CASStore`.

**Independence:** Self-contained. No change to `core`, `redisstore`, or
`pgstore` — the whole point of `core.CASStore`/`core.LockStore` is that a new
backend doesn't touch existing ones. Can be picked up in any order relative
to the other vendor tasks in this folder.

## Scope (ratified — implement exactly this)

- **Module/package:** `mongostore`, module path
  `github.com/asinrus/distributed_go_limiter/mongostore`, with
  `replace github.com/asinrus/distributed_go_limiter/core => ../core` and an
  entry in `go.work`. Mirror `redisstore`'s file layout.
- **Interface:** `core.CASStore`. Rejected `LockStore`: Mongo has no
  hold-a-lock-across-a-round-trip primitive (a transaction on a replica set is
  possible but needs a session, a replica set in the TCK container, and buys
  nothing over a conditional update).
- **Driver:** `go.mongodb.org/mongo-driver/mongo` (official). `NewProxyManager`
  takes a `*mongo.Collection` — not a `*mongo.Client` plus db/collection name
  strings — so namespacing is the caller's, matching `redisstore`'s
  `redis.Cmdable`.
- **Document shape** (one document per bucket key):
  ```
  { _id: <string bucket key>, state: <BinData subtype 0>, expiresAt: <BSON Date | absent> }
  ```
  `_id` is the key, so the primary key index is free — no other index is needed
  for correctness. `expiresAt` absent (not null) means "no expiry".
- **Options:** `WithTTL(time.Duration)` and, matching `pgstore`'s configurable
  names, nothing else. No key prefix option — the collection *is* the namespace.

### CAS mechanics

Both branches are a single `UpdateOne`/`ReplaceOne`; `matchedCount+upsertedCount
== 1` means swapped, `0` means another writer won. **A duplicate-key error
(`mongo.IsDuplicateKeyError`) from the upsert branch is `swapped=false, err=nil`,
not an error** — it is exactly the lost race.

Define one live predicate reused by both reads and writes:
`{ $or: [ {expiresAt: {$exists: false}}, {expiresAt: {$gt: now}} ] }`

- `GetState`: `FindOne({_id: key, <live>})`; `mongo.ErrNoDocuments` →
  `found=false, err=nil`.
- `CompareAndSwap` with `old == nil` (create-only-if-absent):
  `ReplaceOne(filter: {_id: key, $nor: [<live>]}, replacement: <new doc>,
  SetUpsert(true))` — i.e. match only a document that is *absent or expired*.
  Replacing the expired document here is load-bearing: the TTL reaper may not
  have collected it yet, and a plain `InsertOne` would fail forever against a
  stale doc. See `docs/adr/0012-ttl-is-not-a-correctness-mechanism.md`.
- `CompareAndSwap` with `old != nil`:
  `ReplaceOne(filter: {_id: key, state: old, <live>}, replacement: <new doc>)`,
  no upsert. Byte-exact blob compare, same as the Redis Lua script.
- `Remove`: `DeleteOne({_id: key})`, `ErrNoDocuments` is not an error.
- `expiresAt` on every write is `now + ttl`, or omitted when `ttl <= 0`. Use a
  single injectable `now func() time.Time` field like `pgstore.Store.now` so
  tests can move the clock.

### TTL / schema

- Expiry is enforced **in the query predicate above**, which is what makes it
  correct. The TTL index is storage reclamation only (Mongo's reaper runs about
  once a minute).
- **`SchemaSQL` equivalent:** export `IndexModels() []mongo.IndexModel`
  returning the one TTL index (`{expiresAt: 1}`, `SetExpireAfterSeconds(0)`,
  `SetPartialFilterExpression({expiresAt: {$exists: true}})`) plus
  `EnsureIndexes(ctx, *mongo.Collection) error` that applies it — same
  data-then-apply pairing as `pgstore.SchemaSQL`/`EnsureSchema`, generated from
  the same value so the two cannot drift.

### Tests

`tck.Run` against a real container per
`docs/adr/0006-testcontainers-for-backend-tck.md`, using
`testcontainers-go/modules/mongodb` at the version pinned by the other backends
(v0.33.0; `ctr.Terminate(ctx)`, no `TerminateContainer` helper). A standalone
`mongo:7` is enough — nothing here needs a replica set. Keep the
`testing.Short()` skip that `redisstore` uses.

**Acceptance:** `core/tck` suite passes against a real MongoDB container. New
module in `go.work`. `examples/mongodb/` demo mirroring the existing two
(only the `*core.ProxyManager` constructor differs), with its own
`docker-compose.yml`, plus `docs/client/mongodb.md`.

**Status:** Scoped — ready for Dev.
