#!/usr/bin/env bash
# Black-box e2e run for one example backend: brings up its docker-compose
# backend, runs the app with `go run .`, drives it with k6, tears everything
# down. Usage: run-sandbox.sh <backend>, where <backend> is any examples/*
# directory with its own docker-compose.yml.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

BACKEND="${1:-}"
if [[ -z "$BACKEND" || ! -f "$SCRIPT_DIR/../$BACKEND/docker-compose.yml" ]]; then
  available="$(cd "$SCRIPT_DIR/.." && for d in */; do [[ -f "$d/docker-compose.yml" ]] && echo -n "${d%/} "; done)"
  echo "usage: $0 <backend>  (available: $available)" >&2
  exit 1
fi

# Non-interactive shells don't source the podman shell alias, so prefer a
# real `docker` binary if present and fall back to `podman`; DOCKER_BIN
# overrides either.
if [[ -z "${DOCKER_BIN:-}" ]]; then
  DOCKER_BIN="$(command -v docker || command -v podman || true)"
fi
if [[ -z "$DOCKER_BIN" ]]; then
  echo "error: neither docker nor podman found on PATH" >&2
  exit 1
fi

EXAMPLE_DIR="$SCRIPT_DIR/../$BACKEND"
GO_LOG="$(mktemp)"
GO_PID=""

cleanup() {
  local exit_code="$1"
  if [[ -n "$GO_PID" ]]; then
    # `go run` spawns a compiled child process it does not reliably forward
    # plain SIGTERM to; setsid below makes GO_PID a process-group leader so
    # `kill -$GO_PID` reaches both it and the child.
    kill -TERM -- -"$GO_PID" 2>/dev/null || true
    wait "$GO_PID" 2>/dev/null || true
  fi
  (cd "$EXAMPLE_DIR" && "$DOCKER_BIN" compose down -v) || true
  if [[ "$exit_code" != 0 && -s "$GO_LOG" ]]; then
    echo "run failed (exit $exit_code), go run log follows:" >&2
    cat "$GO_LOG" >&2
  fi
  rm -f "$GO_LOG"
}
trap 'cleanup $?' EXIT

(cd "$EXAMPLE_DIR" && "$DOCKER_BIN" compose down -v --remove-orphans) || true

(cd "$EXAMPLE_DIR" && "$DOCKER_BIN" compose up -d --wait)

(cd "$EXAMPLE_DIR" && setsid go run .) >"$GO_LOG" 2>&1 &
GO_PID=$!

PORT_OPEN=""
for _ in $(seq 1 60); do
  if (exec 3<>/dev/tcp/localhost/8080) 2>/dev/null; then
    PORT_OPEN=1
    break
  fi
  sleep 0.5
done
if [[ -z "$PORT_OPEN" ]]; then
  echo "app did not open :8080 in time" >&2
  exit 1
fi

set +e
"$DOCKER_BIN" run --rm --network host -e BASE_URL=http://localhost:8080 \
  -v "$SCRIPT_DIR:/scripts:Z" grafana/k6:2.2.0 run /scripts/ratelimit_test.js
K6_EXIT=$?
set -e

exit "$K6_EXIT"
