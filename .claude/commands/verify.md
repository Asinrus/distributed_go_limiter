---
description: Run the full test suite across all modules (core, redisstore, pgstore)
argument-hint: "[module] — optional: core | redisstore | pgstore"
allowed-tools: Bash, Read, Grep, Glob
---

Run the verification suite for this repo. Module argument (may be empty): $ARGUMENTS

Current podman socket state:
!`test -S "$XDG_RUNTIME_DIR/podman/podman.sock" && echo "socket UP" || echo "socket DOWN — container tests will fail"`

## Environment (required — the toolchain is userspace, not system-installed)

```
export PATH="$HOME/.local/go/bin:$PATH" GOTOOLCHAIN=local
export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/podman/podman.sock" TESTCONTAINERS_RYUK_DISABLED=true
```

If the socket is DOWN, start it before the container-backed modules:
`podman system service --time=0 "unix://$XDG_RUNTIME_DIR/podman/podman.sock" &`

## What to run

If a module was given in $ARGUMENTS, run only that one. Otherwise run all three,
in this order — core first, because it needs no container and a failure there
makes the backend runs meaningless:

Each `cd` is written from the repo root because the shell working directory
**persists between commands** — a bare `cd redisstore` after `cd core` fails.
Let `R="$(git rev-parse --show-toplevel)"`.

1. `cd "$R/core" && go vet ./... && go test -race ./...` — algorithm, local,
   blocking, in-memory TCK. No container.
2. `cd "$R/redisstore" && go test ./...` — testcontainers Redis.
3. `cd "$R/pgstore" && go test ./...` — testcontainers Postgres.

## Constraints — these are the ones that actually bite

- The repo root is a `go.work` workspace, **not** a module. Build and test per
  module (`cd core && go build ./...`), never `go build ./...` from the root.
- Do **not** set `GOFLAGS=-mod=mod` inside the workspace; it errors.
- podman needs `unqualified-search-registries = ["docker.io"]` in
  `~/.config/containers/registries.conf` for the short image names to resolve.

## On failure

Report the failing test name and the AC id it maps to (tests are named
`TestAC<id>_...`, and AC-41..46 are `t.Run` subtests in `core/tck/tck.go`).

Do not fix it inline. Dispatch the **investigator** agent to produce a problem
report at `docs/internal/project/problem_reports/` first — root cause and the
defect-vs-spec-gap routing come before any edit.
