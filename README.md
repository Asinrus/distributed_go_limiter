# distributed_go_limiter

A distributed **token-bucket** rate limiter for Go, rewritten from
[bucket4j](https://github.com/bucket4j/bucket4j) — semantics carried across, not
code transliterated. Buckets share state through a pluggable storage provider —
swap **local ↔ Redis ↔ Postgres ↔ MongoDB ↔ etcd** without changing your
rate-limiting code.

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
b, err := core.NewLocalBucket(cfg)          // in-memory
// pm := redisstore.NewProxyManager(rdb)    // Redis
// pm := pgstore.NewProxyManager(pool)      // Postgres
// pm := mongostore.NewProxyManager(coll)   // MongoDB
// pm := etcdstore.NewProxyManager(cli)     // etcd
// b, err := pm.Bucket("user:42", cfg)
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
  dependency (no `net/http`, no gRPC — an adapter belongs in your code). Also
  ships `core/storetest`, the conformance suite every backend runs against a
  real container — that's what makes the swap claim verifiable, not aspirational.
- `redisstore/` — Redis backend (compare-and-swap via Lua).
- `pgstore/` — Postgres backend (SELECT … FOR UPDATE).
- `mongostore/` — MongoDB backend (conditional `ReplaceOne`).
- `etcdstore/` — etcd backend (`Txn` comparing on `clientv3.Value`, TTL via lease).
- `examples/` — a `net/http` server per backend, with docker-compose files
  (`cd examples/redis && docker compose up -d && go run .`).

## Docs
- [`docs/client/redis.md`](docs/client/redis.md) — TTL and cleanup, Cluster/Sentinel,
  server configuration, what is stored.
- [`docs/client/postgres.md`](docs/client/postgres.md) — schema and how to apply it with
  your own migration tool, sweeping expired rows, server configuration.
- [`docs/client/mongodb.md`](docs/client/mongodb.md) — schema, TTL cleanup, server
  configuration.
- [`docs/client/etcd.md`](docs/client/etcd.md) — options, TTL cleanup, what is stored.
- `ai-context/` — process, roles, and progress (`ai-context/tasks/next-roadmap.md`
  tracker, `ai-context/tasks/` → `ai-context/done/` work items).
- `docs/adr/` — ratified architecture decisions, one file each.
- `CLAUDE.md` — code map + build/test commands.

## Go version
Built and tested at `go 1.25.0`, same version across every module and `go.work`.
The number tracks the highest floor the dependencies force — see
[`docs/adr/0017-one-go-directive-tracks-the-dependency-floor.md`](docs/adr/0017-one-go-directive-tracks-the-dependency-floor.md).

## Status
`v0.1.0`: four backends (Redis, Postgres, MongoDB, etcd) plus the in-memory
reference, single bandwidth per bucket, blocking consume. The five modules are
versioned in lockstep (`docs/adr/0019-v0-1-0-is-five-lockstep-modules-under-apache-2-0.md`).
This is pre-v1.0: the Go compatibility promise doesn't bind yet, and the stored
byte layout is explicitly allowed to change (see each `docs/client/*.md`'s
"what is stored"/"schema" section). See `ai-context/tasks/next-roadmap.md` for
what's next.

## Test
The repo root is a `go.work` workspace (not a module), so build/test per module —
the module list lives in `go.work`, not here:
```
cd core && go test -race ./...       # algorithm + local + core/storetest (no container)
cd redisstore && go test ./...       # testcontainers Redis
cd pgstore && go test ./...          # testcontainers Postgres
cd mongostore && go test ./...       # testcontainers MongoDB
cd etcdstore && go test ./...        # testcontainers etcd
```
The non-`core` modules need a reachable Docker-compatible runtime. For podman:
```
export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/podman/podman.sock"
export TESTCONTAINERS_RYUK_DISABLED=true
```
