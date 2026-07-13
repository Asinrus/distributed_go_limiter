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

## Layout (go.work workspace, 3 modules)
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

## Docs & roles
- `PLANS.md` (root) — progress tracker. **Architect-owned; only the architect edits it.**
- `docs/project/` — design docs: `ARCHITECTURE.md` (mermaid + provider-swap examples),
  `EXT_REDIS.md`, `EXT_POSTGRES.md`, `ACCEPTANCE_CRITERIA.md`, `QUESTIONS_TO_ARCHITECT.md`.
- `docs/bucket4j/` — investigation of the reference library (read-only).

Roles: **Architect** owns `PLANS.md` + `docs/project/` and may dispatch an
**Investigator** (findings → `docs/bucket4j/`). **Dev** implements TDD against
`ACCEPTANCE_CRITERIA.md`. On a verification failure the Investigator writes
`docs/project/problem_reports/<phase>-<n>.md` and triggers Dev (code) or Architect (spec).

## Conventions
- **TDD, tests first.** `ACCEPTANCE_CRITERIA.md` is the contract; each criterion has a
  stable ID (AC-11 … AC-46) referenced from the test name (`TestAC12_...`).
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
