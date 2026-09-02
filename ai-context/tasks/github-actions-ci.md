# CI on GitHub Actions

**Problem:** every gate this project has — `/verify`, the TCK against real
containers, the k6 sandbox — runs on one machine, by hand. Nothing checks a
push.

**Who decided what.** The **owner** said yes to CI on GitHub. The job design
below — module discovery, ordering, which checks run — is the **Architect's**,
derived from what `/verify` already does. The finding that **no test changes**
are needed is also a derivation, not a ruling. The backend tests use testcontainers-go, and
GitHub-hosted Ubuntu runners ship a working Docker daemon, so the containers
come up as they do locally. Rewriting the backend tests onto GHA `services:`
was considered and is not needed. None of the local podman setup applies on a
runner: `DOCKER_HOST` and `TESTCONTAINERS_RYUK_DISABLED` are workarounds for a
podman socket that a runner does not have, and setting them there would break
the run rather than fix it.

**In scope**
- One workflow on push and pull request, Go **1.23** (see
  `ai-context/tasks/go-version-1-23.md`).
- **Discover the modules**, do not hardcode them. `/verify` already does this
  (`.claude/commands/verify.md:32-37`, `go list -f '{{.Dir}}' -m`) and the
  workflow must use the same mechanism, so adding a seventh module never means
  editing CI. A hardcoded module list is the failure this task exists to avoid.
- `core` runs first, with `-race`: it needs no container, and a failure there
  makes every backend run meaningless. Then each remaining module.
- `go vet` per module; `gofmt -l` over the repo, failing on any output;
  `govulncheck` per module.

**Out of scope**
- `golangci-lint` — deliberately. `vet` plus `gofmt` covers the failures this
  repo has actually had, and a linter config is a standing argument about
  style. Revisit if a real defect slips past `vet`.
- The k6 sandbox in CI. It is the tester's gate and needs its own decision
  about runtime and cost; not bundled here.
- Release automation and coverage reporting.

**Done looks like:** a workflow that goes green on a push, whose module list
came from `go list`, that fails loudly on an unformatted file or a known
vulnerability, and that a reader can extend to a new backend by adding nothing.

## Test scenarios

**Black-box (tester, Gate 2):** no black-box surface — no library code changes
and no observable behavior change, so Gate 2 is satisfied by Gate 1
(`docs/adr/0013-architect-scopes-test-coverage.md`). The workflow's own green
run on the pull request is the evidence.

**Unit / integration (Dev):** no new Go tests. Two things must be demonstrated
rather than asserted in prose: the discovered module list on the runner matches
the six modules in `go.work` (print it in the job log), and a deliberately
misformatted file fails the `gofmt` step — a formatting gate that cannot fail
is not a gate.
