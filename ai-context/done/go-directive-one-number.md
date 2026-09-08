# Bring every module back to one `go` directive: 1.25.0

**Problem.** Three modules say `go 1.23` (`core`, `redisstore`, `mongostore`)
and four say `go 1.25.0` (`pgstore`, `etcdstore`, `examples`, `go.work`), while
CI pins `1.25.14`. Nobody can answer "what Go version is this repo" from the
tree. The split is the two-number state the owner explicitly rejected, arrived
at sideways through ADR 0014's per-module escape hatch. That hatch is retired
by `docs/adr/0017-one-go-directive-tracks-the-dependency-floor.md`: one
directive everywhere, equal to the highest floor any module's dependencies
force, which is `go 1.25.0` (`github.com/jackc/pgx/v5` v5.9.2 and
`google.golang.org/grpc` v1.82.1 both declare it).

**Who decided what.** The direction — one number, not two — is the **owner's**,
recorded in ADR 0014 and unchanged. `1.25.0` and the retirement of the
per-module hatch are the **Architect's** derivation in ADR 0017. If the floor
moves again, the number moves without reopening the direction.

**In scope**
- `core/go.mod`, `redisstore/go.mod`, `mongostore/go.mod`: `go 1.23` →
  `go 1.25.0`. Nothing else in those files. No `toolchain` directive anywhere —
  that part of ADR 0014 survives.
- `CLAUDE.md:74` and `README.md:66` both state `go 1.23` and are false. Restate
  the single number, drop the "development and any future consumer alike"
  framing (ADR 0017 explains why the consumer floor was never real), and point
  at ADR 0017 instead of 0014. `CLAUDE.md` is Dev-owned, so this edit belongs
  to whoever does the work.
- **Do not** edit `.github/workflows/ci.yml`. Its `1.25.14` pin is correct and
  ruled on in `ai-context/tasks/github-actions-ci.md`. After this task the
  workflow's inline comment about "core's 1.23" is stale prose, not a wrong
  pin — fix the comment, leave the version.

**Out of scope**
- Dependency upgrades of any kind. This task moves directives, and moving a
  directive must not move a `require` line.
- Raising past `1.25.0` toward the latest release. ADR 0017 rejects that: the
  number tracks the floor, not currency.

**The trap.** Two of them.

1. **Never run `go mod tidy` here.** Inside this workspace it rewrites the `go`
   directive upward rather than reporting a conflict, so it will silently undo
   the point of the task. Edit the directive line by hand.
2. Raising a directive is safer than lowering one (it gates *language
   features*, and no existing code can need a feature from a version newer than
   it was written against), but `go.work` and the six `go.mod` files still have
   to agree afterwards. Verify per module, not with an aggregate green.

**Environment.** `export PATH="$HOME/.local/go/bin:$PATH" GOTOOLCHAIN=local`
and `unset GOROOT` — a mise-provided `GOROOT` shadows the userspace toolchain
and every build fails with `compile: version mismatch` without it. Four agents
have hit this.

**Done looks like:** `go.work` and all six `go.mod` files say `go 1.25.0`, no
`toolchain` line exists, `require` blocks are byte-identical to before,
`CLAUDE.md` and `README.md` state `1.25.0` and cite ADR 0017, and `/verify` is
green with the per-module result reported rather than only the aggregate.

No AC id: this moves a build-configuration number and asserts no library
behavior, so there is nothing for a stable criterion to name. The existing
suite is the whole check.

## Test scenarios

**Black-box (tester, Gate 2):** `e2e/k6/run-sandbox.sh` on all four backends
must exit 0 with the contract unchanged — 5 accepted / 3 rejected in the burst
scenario, all refill-scenario checks passing. No new scenario. The point is
only a compiled-and-running proof that changing the language version changed no
behavior, which is the same evidence the retired
`ai-context/done/go-version-1-23.md` asked for in the opposite direction.

**Unit / integration (Dev):** no new Go tests — a `go` directive has no
behavior to pin. Dev must instead report, per module, that `go build ./...`,
`go vet ./...`, and `go test ./...` pass (`core` with `-race`), because this
task's only real risk is one module failing where the others pass.
