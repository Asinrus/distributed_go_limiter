# Redis configuration

```bash
docker compose up -d
go run .            # PORT=8081 go run . for a second replica
docker compose down
```

Override the address with `REDIS_ADDR` (default `localhost:6379`, matching
[docker-compose.yml](docker-compose.yml)).

## What is Redis-specific here

Everything is in [main.go](main.go); the API in `../api` is untouched.

**Timeouts are not optional.** Every rate-limit check sits between your user and
your handler, so a slow Redis adds its latency to every request. `ReadTimeout` /
`WriteTimeout` of 500ms mean the limiter fails fast and takes the
fail-open/fail-closed branch instead of hanging.

**`WithTTL` is the entire cleanup story.** Redis expires keys itself, so unlike
Postgres there is no sweeper to run. Any TTL comfortably longer than a bucket's
refill time is safe: a bucket that idle has refilled to full anyway, so dropping
it changes nothing.

```console
$ redis-cli TTL ratelimit:search:key:alice
3600
```

**`WithKeyPrefix`** namespaces every key. Worth setting even on a dedicated
instance — it makes limiter keys obvious in `redis-cli` and lets one Redis serve
several apps or environments.

**Cluster and Sentinel work unchanged**, because the backend takes a
`redis.Cmdable`:

```go
rdb := redis.NewClusterClient(&redis.ClusterOptions{Addrs: []string{...}})
rdb := redis.NewFailoverClient(&redis.FailoverOptions{MasterName: "mymaster", ...})
```

One bucket is a single key, so every operation is single-slot on Cluster — no
cross-slot errors, no hash tags needed.

## What is in the compose file, and why

**No persistence** (`--save "" --appendonly no`). Rate-limit state is disposable:
if Redis restarts empty every client just gets a fresh allowance, which is a safe
failure. Skip the fsync cost. Turn RDB back on only if you would rather limits
survive a restart than have faster writes.

**`--maxmemory-policy volatile-ttl`** bounds what a flood of distinct keys can
take. It evicts the soonest-to-expire key first and only ever touches keys that
*have* a TTL — which is why the app must set `WithTTL`. Without TTLs this policy
has nothing to evict and writes fail with OOM.

Eviction hands the evicted client a fresh allowance, so size `maxmemory` so it
does not happen routinely: roughly 100 bytes per bucket, one bucket per
(scope, client).

## How the state looks

25 bytes per bucket — a version byte plus three int64s. The token maths runs in
`core`, never in Redis; the only thing Redis provides is atomic compare-and-swap
(a Lua script) and `SET NX`.

```console
$ redis-cli KEYS 'ratelimit:*'
ratelimit:search:key:alice
$ redis-cli STRLEN ratelimit:search:key:alice
25
```
