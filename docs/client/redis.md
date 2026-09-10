# Redis backend

```go
rdb := redis.NewClient(&redis.Options{Addr: addr})
pm := redisstore.NewProxyManager(rdb, redisstore.WithTTL(time.Hour))
```

That is the entire Redis-specific surface. Everything after it — `pm.Bucket(key, cfg)`,
`b.TryConsume(ctx, 1)` — is the same `core.ProxyManager` API the Postgres backend uses, and
the same one `core.NewInMemoryProxyManager()` gives you with no container at all.

## Options

| Option | Default | Effect |
|---|---|---|
| `WithKeyPrefix(p)` | `""` | Namespaces bucket keys (e.g. `"dgl:"`). |
| `WithTTL(d)` | `0` (no expiry) | Expire-after-write on the bucket key. Sugar for `WithProxyOption(core.WithExpireAfterWrite(d))`. |
| `WithProxyOption(o)` | — | Forwards a `core.ProxyOption` (`core.WithClock`, `core.WithMaxCASRetries`, `core.WithExpireAfterWrite`, …) to the underlying `core.ProxyManager`. |

## `WithTTL` is the whole cleanup story

Redis expires the key itself, so unlike Postgres there is nothing to sweep. Any TTL
comfortably longer than a bucket's refill time is safe — a bucket idle that long has already
refilled to full, so dropping it changes nothing.

Without a TTL, every client you ever rate-limited stays in Redis forever.

## Cluster and Sentinel

Both work unchanged: the backend takes a `redis.Cmdable`, not a concrete client type.

```go
rdb := redis.NewClusterClient(&redis.ClusterOptions{Addrs: []string{...}})
rdb := redis.NewFailoverClient(&redis.FailoverOptions{MasterName: "mymaster", ...})
```

One bucket is a single key, so every operation is single-slot on Cluster — no cross-slot
errors, no hash tags needed.

## Server configuration

**Persistence is optional.** Rate-limit state is disposable: if Redis restarts empty, every
client just gets a fresh allowance — a safe failure. Running with `--save "" --appendonly no`
skips the fsync cost. Turn RDB back on only if you would rather limits survive a restart than
have faster writes.

**`--maxmemory-policy volatile-ttl`** bounds what a flood of distinct keys can take. It evicts
the soonest-to-expire key first and only touches keys that *have* a TTL — which is why the
application must set `WithTTL`. Without TTLs this policy has nothing to evict and writes fail
with OOM instead.

Eviction hands the evicted client a fresh allowance, so size `maxmemory` such that it does not
happen routinely: roughly 100 bytes per bucket, one bucket per client.

## What is stored

25 bytes per bucket — a version byte plus three big-endian int64s. The token maths runs in
`core`, never in Redis; Redis provides only atomic compare-and-swap (a Lua script) and
`SET NX`.

```console
$ redis-cli KEYS '*'
127.0.0.1
$ redis-cli STRLEN '127.0.0.1'
25
$ redis-cli TTL '127.0.0.1'
3600
```

## Errors

Any Redis failure — I/O, timeout, cancellation — comes back wrapping `core.ErrStore`, with the
`go-redis` error still reachable underneath via `errors.Is`/`errors.As`. `core` does not pick
fail-open or fail-closed for you:

```go
ok, err := b.TryConsume(ctx, 1)
if errors.Is(err, core.ErrStore) {
    // your call: serve anyway (fail open) or reject (fail closed)
}
```
