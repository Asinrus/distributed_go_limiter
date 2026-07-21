# CLAUDE.md

Guidance for working in this repository.

## What this is
A distributed **token-bucket** rate limiter for Go, ported from
[bucket4j](https://github.com/bucket4j/bucket4j). Buckets share state through a
pluggable storage provider. **Primary design goal: swap the provider
(local ↔ Redis ↔ Postgres) without changing any rate-limiting logic** — the
`core.Bucket` API and all calling code stay identical; only the constructor differs.

Key idea inherited from bucket4j: **the token math never runs in the store.** The
store is a dumb byte-blob KV with one concurrency primitive (compare-and-swap, or a
row lock). The client reads bytes → runs the math in `core` → writes bytes back
atomically. So the algorithm lives once, in `core`.

## Process & context
Roles, process, and progress live in **`ai-context/`** (start with
`ai-context/README.md`; `ai-context/PLANS.md` is the architect-owned progress
tracker). Detailed design and investigation notes are kept local under
`docs/internal/` (untracked).

## Layout (go.work workspace, 4 modules)
Root is a `go.work` workspace, **not** a module — build/test per module.

- `core/` — API + algorithm, **zero external runtime deps**.
  - `state.go` — the token-bucket math (single bandwidth): 3 int64s
    `[lastRefillNanos, currentSize, roundingError]`, greedy rounding-error carry +
    intervally boundaries. Ported from bucket4j `BucketState64BitsInteger`.
  - `ops.go` — operations on state (tryConsume/estimate/reserve/…) shared by the
    local and distributed paths so the math is written once.
  - `bucket.go` — the `Bucket` interface (ctx + error; local ignores both).
  - `local.go` — `LocalBucket` (mutex, in-memory hot path).
  - `blocking.go` — `BlockingBucket.Consume` (reservation-based wait, no busy-spin).
  - `store.go` — provider contracts: `CASStore` (Redis/in-memory) and `LockStore` (Postgres).
  - `command.go` — the read-modify-write executor (CAS retry loop / lock callback).
  - `proxymanager.go` — `ProxyManager` (same concrete type for every backend).
  - `codec.go` — deterministic state ⇄ []byte (for CAS byte-equality).
  - `memstore.go` — in-memory `CASStore` + `NewInMemoryProxyManager` (reference impl).
  - `tck/` — shared conformance suite (stdlib `testing` only, no deps). Every backend
    runs the SAME suite; this is how provider-swap (AC-46) is proven.
- `redis/` (package `redisext`) — `CASStore` via Lua compare-and-swap + `SET NX`.
- `postgres/` (package `pgext`) — `LockStore` via `SELECT … FOR UPDATE`.
- `examples/` — a Gin API wired to each backend. `examples/api/` is the shared
  HTTP layer (router + rate-limit middleware) and must stay **provider-agnostic**;
  `examples/redis/` and `examples/postgres/` are `main` packages that differ only
  in how they build the `*core.ProxyManager`, each with a `docker-compose.yml`.
  If a change forces a provider-specific branch into `api/`, the swap goal has
  regressed. Needs Go 1.25 (gin), hence the `go.work` floor; the three library
  modules stay on 1.23.

## Conventions
- **TDD, tests first.** The acceptance criteria (in `ai-context/`) are the contract;
  each criterion has a stable ID (AC-11 … AC-46) referenced from the test name
  (`TestAC12_...`). The shared `core/tck` suite is their executable form.
- **v1 = single bandwidth per bucket.** Deferred: multi-bandwidth, async, verbose
  diagnostics, optimization/batching, config replacement + versioning, grid providers.
- New distributed backend = implement `core.CASStore` **or** `core.LockStore`, then
  `core.NewCASProxyManager` / `core.NewLockProxyManager`, then run `tck.Run` against it.
- Time source: local buckets use a monotonic clock; distributed use wall-clock epoch
  (shared across nodes). Both injectable via `WithClock` for tests.

## Build & test
```bash
export PATH="$HOME/.local/go/bin:$PATH" GOTOOLCHAIN=local   # Go is userspace, no system install
cd core && go test -race ./...        # algorithm + local + blocking + in-memory TCK (no container)
cd ../redis && go test ./...          # testcontainers Redis
cd ../postgres && go test ./...       # testcontainers Postgres
```
- Build **per module** (`cd core && go build ./...`), not `go build ./...` from root.
- Do NOT set `GOFLAGS=-mod=mod` inside the workspace (it errors).
- Redis/Postgres tests need a Docker-compatible runtime. With podman:
  ```bash
  export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/podman/podman.sock" TESTCONTAINERS_RYUK_DISABLED=true
  # start once: podman system service --time=0 "unix://$XDG_RUNTIME_DIR/podman/podman.sock" &
  ```
  `~/.config/containers/registries.conf` needs `unqualified-search-registries = ["docker.io"]`.
- `testcontainers-go` v0.33.0 has no `TerminateContainer` helper — use `ctr.Terminate(ctx)`.
