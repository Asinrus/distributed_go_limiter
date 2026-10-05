# MongoDB backend

```go
client, _ := mongo.Connect(ctx, options.Client().ApplyURI(uri))
coll := client.Database("limiter").Collection("bucket")
pm := mongostore.NewProxyManager(coll, mongostore.WithTTL(time.Hour))
```

That is the entire MongoDB-specific surface. Everything after it — `pm.Bucket(key, cfg)`,
`b.TryConsume(ctx, 1)` — is the same `core.ProxyManager` API every backend uses, including
`core.NewInMemoryProxyManager()` with no container at all.

## Options

| Option | Default | Effect |
|---|---|---|
| `WithTTL(d)` | `0` (no expiry) | Marks state stale after this long. Sugar for `WithProxyOption(core.WithExpireAfterWrite(d))`. See cleanup below. |
| `WithProxyOption(o)` | — | Forwards a `core.ProxyOption` (`core.WithClock`, `core.WithMaxCASRetries`, `core.WithExpireAfterWrite`, …) to the underlying `core.ProxyManager`. |

There is no collection-name option: `NewProxyManager` takes a `*mongo.Collection`, so the
collection itself is the namespace, matching `redisstore`'s `redis.Cmdable` shape.

## Schema

One document per bucket:

```json
{ "_id": "<bucket key>", "state": BinData(0, "..."), "expiresAt": ISODate("...") }
```

`state` is 25 bytes — a version byte plus three big-endian int64s, `core`'s fixed wire format
(same across every backend; see `core/codec.go`). The token maths runs in `core`; MongoDB
provides only the conditional replace (`ReplaceOne` with a filter on `_id`
and, for a CAS, the previous `state`). `expiresAt` is absent, not null, when the document has
no expiry.

This layout is a private wire format between this library and itself, not a
public interface, and it will change before v1.0 — multi-bandwidth needs N
states where there is now one. The version byte makes that safe: `decodeState`
rejects a byte count or version it doesn't recognize rather than misreading it,
so a mixed-version fleet fails loudly on the affected documents. Don't parse
`state` or migrate it by hand; let the documents drain or expire across an
upgrade that changes the format.

### Applying it

```go
if err := mongostore.EnsureIndexes(ctx, coll); err != nil { ... }
```

Creates the one TTL index (`{expiresAt: 1}`, partial on `expiresAt` existing). Idempotent,
safe to call on every start. `mongostore.IndexModels()` returns the same index as a
`[]mongo.IndexModel`, for callers who apply their own index management instead.

## Cleanup — MongoDB's TTL reaper is not instant

Unlike Redis, MongoDB's TTL index is reclaimed by a background sweep that runs roughly once a
minute, not the instant the document expires. `mongostore` does not rely on it for
correctness: every read and write filters on a stored `expiresAt` field, so a stale document
reads as "absent" long before the reaper collects it. The TTL index only reclaims storage. See
`docs/adr/0012-ttl-is-not-a-correctness-mechanism.md`.

Without `WithTTL`, `expiresAt` is never set and no document is ever reclaimed — every client
you ever rate-limited stays in the collection forever, same as Redis without a TTL.

## Errors

Any MongoDB failure — I/O, timeout, cancellation — comes back wrapping `core.ErrStore`, with
the underlying driver error still reachable via `errors.Is`/`errors.As`. `core` does not pick
fail-open or fail-closed for you:

```go
ok, err := b.TryConsume(ctx, 1)
if errors.Is(err, core.ErrStore) {
    // your call: serve anyway (fail open) or reject (fail closed)
}
```
