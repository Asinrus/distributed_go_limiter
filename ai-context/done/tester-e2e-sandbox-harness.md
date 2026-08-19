# Build the black-box e2e sandbox for the tester agent

**Origin:** `docs/adr/0011-tester-is-black-box-only.md`. The tester's
end-to-end gate must run the actual compiled application and check observed
HTTP behavior, not run `go test` or read source/DB state. This task builds
the harness that makes that possible; the tester agent itself will be
updated separately to use it once this exists and has been proven to work.

**Goal:** One k6 script plus one shell runner, reusable by both
`examples/redis` and `examples/postgres` (they expose an identical HTTP
contract — capacity 5, greedy refill 5/min, `GET /` returns `200` or `429`).

1. `examples/k6/ratelimit_test.js` — a k6 script, parameterized by
   `BASE_URL` env var (default `http://localhost:8080`). Single VU,
   `iterations: 8` (> the bucket's capacity of 5, small enough this is a
   correctness check, not a load test), executed sequentially so request
   order is deterministic. For each iteration: `GET {BASE_URL}/`, check the
   status is `200` or `429` (anything else is an error), and tally into two
   custom `Counter` metrics, `accepted_total` and `rejected_total`. Set
   `thresholds` so the run fails (non-zero exit) unless
   `accepted_total == 5` and `rejected_total == 3` — the exact split the
   bucket config guarantees for 8 sequential requests with no time gap.

2. `examples/k6/run-sandbox.sh <redis|postgres>` — orchestrates one full
   black-box run for the named backend example:
   - `cd examples/<backend> && docker compose up -d --wait` (bring up the
     real backend container; `--wait` blocks on the existing healthcheck).
   - `go run .` in the background, capture its PID and stdout/stderr to a
     temp log file.
   - Poll (short retry loop, few seconds max) until the app's port
     (`localhost:8080`) accepts a TCP connection — do not just `sleep N`.
   - Run the k6 script against it via the official image, no local k6
     install assumed:
     `docker run --rm --network host -e BASE_URL=http://localhost:8080 -v "$(pwd)/../k6:/scripts" grafana/k6 run /scripts/ratelimit_test.js`
   - **Always** clean up on any exit path (success, k6 failure, or script
     error): kill the backgrounded `go run` process, `docker compose down
     -v`. Use a `trap` for this, not a linear kill-at-the-end — a failure
     partway through must not leak the app process or the container.
   - Exit with k6's exit code.

**Independence:** Self-contained under `examples/`. No change to `core`,
`redisstore`, `pgstore`, or either example's `main.go`/`docker-compose.yml`
— the harness only drives them from outside.

**Acceptance:** Running `examples/k6/run-sandbox.sh postgres` and
`examples/k6/run-sandbox.sh redis` each exit 0 against the current (correct)
code, with k6 reporting `accepted_total==5` / `rejected_total==3`. To prove
the harness actually detects a real regression and not just a script that
always exits 0: temporarily change one example's bucket capacity (e.g.
`core.NewBandwidth(5)` → `core.NewBandwidth(3)`) and confirm the run now
fails (thresholds don't match 5/3 anymore) — then revert the change before
finishing, it's a verification step, not a real code change for this task.

This task builds tooling only and does not itself re-gate any other branch.
Do NOT edit `ai-context/PLANS.md` or `docs/adr/` beyond the ADR this task's
own ruling requires.

**Amendment:** `.claude/agents/tester.md` and `.claude/agents/dev.md` *are*
edited as part of this change, despite the line above — a first pass left
the harness built but the agent definitions unchanged, which made ADR
0011's present-tense claims ("the tester now treats the system as a black
box") false until a companion edit landed them. Bundling both into this same
change keeps the ADR's claims true within the diff that introduces it,
rather than describing a state that doesn't exist yet.

**Bootstrap exception, moved to `done/` without a tester pass:** this task
*builds* the tester's Gate 2, so the tester agent can't gate it (there is no
prior sandbox to run). In its place: six rounds of dev implementation + code
review, each verified against real `run-sandbox.sh postgres`/`redis` runs
(including a deliberate-regression proof, a deliberate-crash-leftover-state
proof, and a forced-early-failure proof) — moved here on that basis, not a
tester's independent confirmation.
