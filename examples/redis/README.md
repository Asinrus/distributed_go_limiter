# Redis configuration

```bash
docker compose up -d
go run .                     # PORT=8081 go run . for a second, concurrent replica
docker compose down
```

Override the address with `REDIS_ADDR` (default `localhost:6379`, matching
[docker-compose.yml](docker-compose.yml)).

## What [main.go](main.go) actually does

```go
rdb := redis.NewClient(&redis.Options{Addr: addr})
pm := redisstore.NewProxyManager(rdb, redisstore.WithTTL(time.Hour))
```

That's the entire Redis-specific part. Everything after it — `pm.Bucket(key,
cfg)`, `b.TryConsume(ctx, 1)` — is the same `core.ProxyManager` API the Postgres
example uses, and the same one `core.NewInMemoryProxyManager()` gives you with no
container at all.

**`WithTTL` is the whole cleanup story.** Redis expires the key itself, so unlike
Postgres there is nothing to sweep. Any TTL comfortably longer than a bucket's
refill time is safe — a bucket that idle has already refilled to full, so
dropping it changes nothing.

```console
$ redis-cli TTL '127.0.0.1'
3600
```

**Cluster and Sentinel work unchanged**, because the backend takes a
`redis.Cmdable`, not a concrete client type:

```go
rdb := redis.NewClusterClient(&redis.ClusterOptions{Addrs: []string{...}})
rdb := redis.NewFailoverClient(&redis.FailoverOptions{MasterName: "mymaster", ...})
```

One bucket is a single key, so every operation is single-slot on Cluster — no
cross-slot errors, no hash tags needed.

## What is in the compose file, and why

**No persistence** (`--save "" --appendonly no`). Rate-limit state is disposable:
if Redis restarts empty, every client just gets a fresh allowance — a safe
failure. Skip the fsync cost. Turn RDB back on only if you would rather limits
survive a restart than have faster writes.

**`--maxmemory-policy volatile-ttl`** bounds what a flood of distinct keys can
take. It evicts the soonest-to-expire key first and only touches keys that
*have* a TTL — which is why the app sets `WithTTL`. Without TTLs this policy has
nothing to evict and writes fail with OOM instead.

Eviction hands the evicted client a fresh allowance, so size `maxmemory` so it
does not happen routinely: roughly 100 bytes per bucket, one bucket per client.

## How the state looks

25 bytes per bucket — a version byte plus three int64s. The token maths runs in
`core`, never in Redis; the only thing Redis provides is atomic compare-and-swap
(a Lua script) and `SET NX`.

```console
$ redis-cli KEYS '*'
127.0.0.1
$ redis-cli STRLEN '127.0.0.1'
25
```
