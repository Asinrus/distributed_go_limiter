# distributed_go_limiter

A distributed **token-bucket** rate limiter for Go, ported from
[bucket4j](https://github.com/bucket4j/bucket4j). Buckets share state through a
pluggable storage provider — swap **local ↔ Redis ↔ Postgres** without changing
your rate-limiting code.

```go
func handle(ctx context.Context, b core.Bucket) {
    if ok, err := b.TryConsume(ctx, 1); err == nil && ok {
        serve()
    } else {
        rejectHTTP429()
    }
}

cfg := core.NewConfig(core.NewBandwidth(50).RefillGreedy(10, time.Second))

// The config is validated once, here — every bucket that exists is usable.
b, err := core.NewLocalBucket(cfg)                                // in-memory
// b, err := redisstore.NewProxyManager(rdb).Bucket("user:42", cfg)  // Redis
// b, err := pgstore.NewProxyManager(pool).Bucket("user:42", cfg)    // Postgres
```

## One budget across every replica

That is the property an in-process limiter cannot give you. Two independent processes,
one shared bucket of 5:

```console
$ cd examples/redis && docker compose up -d
$ PORT=8080 go run . &  PORT=8081 go run . &
$ for p in 8080 8081 8080 8081 8080 8081; do
    curl -s -o /dev/null -w "$p -> %{http_code}\n" localhost:$p/
  done
8080 -> 200
8081 -> 200
8080 -> 200
8081 -> 200
8080 -> 200
8081 -> 429      <-- 6th request over the limit, whichever process gets it
```

With `golang.org/x/time/rate`, or a `map[string]*rate.Limiter` held in-process, each
replica keeps its own count and doubling the replicas silently doubles the real limit.

## Modules
- `core/` — API + token-bucket algorithm, zero external runtime deps, no transport
  dependency (no `net/http`, no gRPC — an adapter belongs in your code).
- `redisstore/` — Redis backend (compare-and-swap via Lua).
- `pgstore/` — Postgres backend (SELECT … FOR UPDATE).
- `examples/` — a `net/http` server per backend, with docker-compose files
  (`cd examples/redis && docker compose up -d && go run .`).

## Docs
- [`docs/client/redis.md`](docs/client/redis.md) — TTL and cleanup, Cluster/Sentinel,
  server configuration, what is stored.
- [`docs/client/postgres.md`](docs/client/postgres.md) — schema and how to apply it with
  your own migration tool, sweeping expired rows, server configuration.
- `ai-context/` — process, roles, and progress (`ai-context/tasks/next-roadmap.md`
  tracker, `ai-context/tasks/` → `ai-context/done/` work items; see
  `docs/adr/0016-retire-plans-md.md`).
- `docs/adr/` — ratified architecture decisions, one file each.
- `CLAUDE.md` — code map + build/test commands.

## Go version
Built and tested at `go 1.23`, same version across every module.

## Status
v1: single bandwidth per bucket, blocking consume, three backends. See `ai-context/tasks/next-roadmap.md`.

## Test
The repo root is a `go.work` workspace (not a module), so build/test per module:
```
cd core && go test -race ./...       # algorithm + local + in-memory TCK (no container)
cd redisstore && go test ./...       # testcontainers Redis
cd pgstore && go test ./...          # testcontainers Postgres
```
The Redis/Postgres tests need a reachable Docker-compatible runtime. For podman:
```
export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/podman/podman.sock"
export TESTCONTAINERS_RYUK_DISABLED=true
```
