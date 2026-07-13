# distributed_go_limiter

A distributed **token-bucket** rate limiter for Go, ported from
[bucket4j](https://github.com/bucket4j/bucket4j). Buckets share state through a
pluggable storage provider — swap **local ↔ Redis ↔ Postgres** without changing
your rate-limiting code.

```go
func handle(b core.Bucket) {
    if b.TryConsume(1) { serve() } else { rejectHTTP429() }
}

cfg := core.NewConfig(core.NewBandwidth(50).RefillGreedy(10, time.Second))

b := core.NewLocalBucket(cfg)              // in-memory
// b := redisext.NewProxyManager(rdb).Bucket("user:42", cfg)   // Redis
// b := pgext.NewProxyManager(pool).Bucket("user:42", cfg)     // Postgres
```

## Modules
- `core/` — API + token-bucket algorithm, zero external runtime deps.
- `redis/` — Redis backend (compare-and-swap via Lua).
- `postgres/` — Postgres backend (SELECT … FOR UPDATE).

## Docs
- `ai-context/` — process, roles, and progress (`ai-context/PLANS.md` tracker).
- `CLAUDE.md` — code map + build/test commands.
- Detailed design & investigation notes are kept local under `docs/internal/` (untracked).

## Status
v1: single bandwidth per bucket, blocking consume, three backends. See `ai-context/PLANS.md`.

## Test
The repo root is a `go.work` workspace (not a module), so build/test per module:
```
cd core && go test -race ./...   # algorithm + local + in-memory TCK (no container)
cd redis && go test ./...        # testcontainers Redis
cd postgres && go test ./...     # testcontainers Postgres
```
The Redis/Postgres tests need a reachable Docker-compatible runtime. For podman:
```
export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/podman/podman.sock"
export TESTCONTAINERS_RYUK_DISABLED=true
```
