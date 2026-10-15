# CLAUDE.md

Guidance for working in this repository.

## What this is
A distributed **token-bucket** rate limiter for Go, rewritten from
[bucket4j](https://github.com/bucket4j/bucket4j) — its semantics carried across,
not its code transliterated, so a difference from the Java is not by itself a bug
(see `docs/adr/0018-deliberate-deviations-from-bucket4j.md`). Buckets share
state through a pluggable store. **Primary design goal: swap the store
(local ↔ Redis ↔ Postgres ↔ MongoDB ↔ etcd) without changing any rate-limiting
logic** — the `core.Bucket` API and all calling code stay identical; only the
constructor differs.

Key idea inherited from bucket4j: **the token math never runs in the store.** The
store is a dumb byte-blob KV with one concurrency primitive (compare-and-swap, or a
row lock). The client reads bytes → runs the math in `core` → writes bytes back
atomically. So the algorithm lives once, in `core`.

## Process & context
Roles, process, and progress live in **`ai-context/`** (start with
`ai-context/README.md`; `ai-context/tasks/next-roadmap.md` is the current
architect-owned progress tracker). User-facing backend docs are in
`docs/client/`. Ratified decisions
are recorded as ADRs in `docs/adr/`; work items track through
`ai-context/tasks/` → `ai-context/done/`.

## Vocabulary
Three words, used precisely; they are not interchangeable.

- **vendor** — the external system (Redis, Postgres, MongoDB, etcd).
- **store** — our adapter implementing `core.CASStore` / `core.LockStore`.
- **backend** — the Go module shipping one (`redisstore/`).

"Provider" is retired except inside the ratified **provider-swap** / AC-46
phrase, which is frozen wording. (`vendor/` is a reserved Go directory name;
harmless, since the word is used in prose and identifiers only.)

## Layout (go.work workspace, 6 modules)
Root is a `go.work` workspace, **not** a module — build/test per module.

- `core/` — API + algorithm, **zero external runtime deps**. `core` is also
  **transport-agnostic**: no `net/http`, no gRPC, no middleware. An adapter belongs
  in the caller; store failures surface as `core.ErrStore` and the caller picks
  fail-open vs fail-closed.
  - `state.go` — the token-bucket math (single bandwidth): 3 int64s
    `[lastRefillNanos, currentSize, roundingError]`, greedy rounding-error carry +
    intervally boundaries. Reduced from bucket4j's `BucketState64BitsInteger`.
  - `ops.go` — operations on state (tryConsume/estimate/reserve/…) shared by the
    local and distributed paths so the math is written once.
  - `bucket.go` — the `Bucket` interface (ctx + error; `LocalBucket` ignores ctx
    and returns only argument/config errors, never `ErrStore`).
  - `local.go` — `LocalBucket` (mutex, in-memory hot path).
  - `blocking.go` — `BlockingBucket.Consume` (reservation-based wait, no busy-spin).
  - `store.go` — the two store contracts: `CASStore` (Redis, MongoDB, etcd,
    in-memory) and `LockStore` (Postgres).
  - `command.go` — the read-modify-write executor (CAS retry loop / lock callback).
  - `proxymanager.go` — `ProxyManager` (same concrete type for every backend).
  - `codec.go` — deterministic state ⇄ []byte (for CAS byte-equality).
  - `memstore.go` — in-memory `CASStore` + `NewInMemoryProxyManager` (reference impl).
  - `storetest/` — shared conformance suite (stdlib `testing` only, no deps). Every backend
    runs the SAME suite; this is how provider-swap (AC-46) is proven.
- `redisstore/` (package `redisstore`) — `CASStore` via Lua compare-and-swap + `SET NX`.
- `pgstore/` (package `pgstore`) — `LockStore` via `SELECT … FOR UPDATE`.
  `SchemaSQL` exposes the DDL for users who run their own migrations; `EnsureSchema`
  applies it directly.
- `mongostore/` (package `mongostore`) — `CASStore` via a conditional `ReplaceOne`.
- `etcdstore/` (package `etcdstore`) — `CASStore` via `Txn` comparing on
  `clientv3.Value`; expiry rides an etcd lease.
- `examples/` — one `main` package per backend (`examples/redis/`, `postgres/`,
  `mongodb/`, `etcd/`), each with a `docker-compose.yml`. **The `net/http` lives here, in the demo programs —
  not in `core`.** Only backend-specific line is the `*core.ProxyManager` constructor;
  everything after that (`pm.Bucket`, `TryConsume`) is identical. Deliberately
  un-abstracted — no shared package between them — so the library call is never one
  hop away from `main()`.

## Conventions
- **TDD, tests first.** The acceptance criteria are the contract; each has a
  stable ID (`AC-11` upward; `AC-61` is the highest minted) referenced from the
  test name (`TestAC12_...`). There is no standing AC registry —
  `grep -rn 'AC-'` is the index — and the shared `core/storetest` suite is their
  executable form.
- **Comments earn their place.** Godoc on every exported symbol, plus comments that
  explain a *why* the code cannot. No restating the code, no tutorial narrative.
  A deliberate trade-off gets a comment naming its limit and what would justify
  revisiting it.
- **v1 = single bandwidth per bucket.** Deferred: multi-bandwidth, async, verbose
  diagnostics, optimization/batching, config replacement + versioning, grid providers.
- New distributed backend = implement `core.CASStore` **or** `core.LockStore`, then
  `core.NewCASProxyManager` / `core.NewLockProxyManager`, then run `storetest.Run` against it.
  A backend with store-level settings of its own defines an `Option` type and a
  `WithProxyOption` to forward core's (redisstore, pgstore, etcdstore); one with none
  takes `core.ProxyOption` directly (mongostore). Both shapes are correct.
- Time source: local buckets use a monotonic clock; distributed use wall-clock epoch
  (shared across nodes). The distributed clock is injectable via `WithClock`
  (a `core.ProxyOption`); `NewLocalBucket` takes no options — the local path
  injects through the unexported `newLocalBucketClock`, for tests in `core` only.

## Go version
One version, `go 1.25.0`, for every `go.mod` and `go.work` — no per-module
exception, no `toolchain` directive. The number tracks the highest floor any
module's dependencies force, so it moves only when an upgrade forces it. See
`docs/adr/0017-one-go-directive-tracks-the-dependency-floor.md`.

## Build & test
`/verify` discovers the workspace modules, runs the suite, and routes failures to the
investigator — the block below is what it does, and the fallback.
```bash
# Go is userspace; a stale GOROOT/GOBIN from a version manager fails every build
# with an error that does not name itself as the cause.
unset GOROOT GOBIN && export PATH="$HOME/.local/go/bin:$PATH" GOTOOLCHAIN=local
cd core && go test -race ./...        # algorithm + local + blocking + in-memory storetest (no container)
cd ../redisstore && go test ./...     # testcontainers Redis
cd ../pgstore && go test ./...        # testcontainers Postgres
cd ../mongostore && go test ./...     # testcontainers MongoDB
cd ../etcdstore && go test ./...      # testcontainers etcd
cd ../examples && go build ./...      # main packages, no tests
```
- Build **per module** (`cd core && go build ./...`), not `go build ./...` from root.
- Do NOT set `GOFLAGS=-mod=mod` inside the workspace (it errors).
- The four container-backed modules need a Docker-compatible runtime. With podman:
  ```bash
  export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/podman/podman.sock" TESTCONTAINERS_RYUK_DISABLED=true
  # start once: podman system service --time=0 "unix://$XDG_RUNTIME_DIR/podman/podman.sock" &
  ```
  `~/.config/containers/registries.conf` needs `unqualified-search-registries = ["docker.io"]`.
- `testcontainers-go` v0.33.0 has no `TerminateContainer` helper — use `ctr.Terminate(ctx)`.
