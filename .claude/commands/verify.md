---
description: Run the full test suite across every module in the go.work workspace
argument-hint: "[module] — optional: a module directory name; default is all"
allowed-tools: Bash, Read, Grep, Glob
---

Run the verification suite for this repo. Module argument (may be empty): $ARGUMENTS

Container runtime state:
!`test -n "$DOCKER_HOST" || test -S /var/run/docker.sock && echo "runtime configured" || echo "NO container runtime — container-backed modules will fail"`

## Environment

The Go toolchain is userspace, not system-installed:

```
export PATH="$HOME/.local/go/bin:$PATH" GOTOOLCHAIN=local
```

Container-backed tests need a Docker-compatible runtime reachable via `DOCKER_HOST`
or the default socket. On this machine that is podman:

```
export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/podman/podman.sock" TESTCONTAINERS_RYUK_DISABLED=true
# start once: podman system service --time=0 "unix://$XDG_RUNTIME_DIR/podman/podman.sock" &
```

Any other Docker-compatible runtime works; nothing in the suite is podman-specific.

## What to run

Discover the modules — do not hardcode the list, it changes:

```
R="$(git rev-parse --show-toplevel)"
go list -f '{{.Dir}}' -m
```

Run `core` first: it needs no container, and a failure there makes every backend run
meaningless. Then the rest, in any order. If `$ARGUMENTS` names a module, run only that one.

Per module, from the repo root each time — the shell working directory **persists between
commands**, so a bare `cd redisstore` after `cd core` fails:

```
cd "$R/<module>" && go vet ./... && go test ./...
```

Add `-race` for `core` (it is the only module with in-process concurrency under test).
Modules that are `main` packages with no tests (`examples`) only need `go build ./...`.

## Constraints — these are the ones that actually bite

- The repo root is a `go.work` workspace, **not** a module. Build and test per module
  (`cd core && go build ./...`), never `go build ./...` from the root.
- Do **not** set `GOFLAGS=-mod=mod` inside the workspace; it errors.
- With podman, short image names need
  `unqualified-search-registries = ["docker.io"]` in `~/.config/containers/registries.conf`.

## On failure

Report the failing test name and the AC id it maps to (tests are named `TestAC<id>_...`;
AC-41..44 are `t.Run` subtests in `core/tck/tck.go`, run identically by every backend).

Do not fix it inline. Dispatch the **investigator** agent to produce a problem report at
`docs/internal/project/problem_reports/` first — root cause and the defect-vs-spec-gap
routing come before any edit.
