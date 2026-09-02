# Pin the project to Go 1.23

**Who decided what.** The **owner** set the direction: one Go version for
development and consumers alike, chosen as the most widely used version — no
separate development pin. The number **1.23**, and the reasoning below, are the
**Architect's** derivation from the measured dependency floors. A later reader
should be able to tell the decision from the derivation, so: if the floors move,
the number moves without reopening the direction.

A consumer minimum differing from the version contributors build with was
considered and rejected — two numbers to keep honest, and the difference buys
nothing here. No `toolchain` directive.

**Why this matters in a repo with no consumers.** The owner has since ruled that
this is a portfolio / reference implementation, not a near-term release
(`ai-context/tasks/next-roadmap.md`), so do not justify this with consumer
reach — there are no consumers. Two reasons that do survive that: the version
local runs and CI agree on is the version a failure can be reproduced at, and
a repo whose purpose is to be read should demonstrate the practice it is
describing. That is the whole case. It is enough to do the work and not enough
to do more than the work.

**Why 1.23 and not something else.** Measured per module with `GOWORK=off`,
counting third-party dependencies only, the highest floor any dependency
forces is `etcdstore` at **go1.22** (`core` none — it has zero third-party
runtime deps; `redisstore` go1.18; `mongostore` go1.20; `pgstore` go1.21).
1.23 clears all of them and is the most-used version in production Go today.

An earlier reading of this put the floor at 1.24. That figure was an artifact
of `go.work`: the workspace unifies `golang.org/x/text` up to the v0.34.0 that
the `examples` module pulls in, and the floor has to be measured with the
workspace off. Do not re-derive it inside the workspace.

Today every `go.mod` and `go.work` says `go 1.26.5`.

**In scope**
- `go.work` and the six `go.mod` files (`core`, `redisstore`, `pgstore`,
  `mongostore`, `etcdstore`, `examples`) set to `go 1.23`. No `toolchain` line.
- `CLAUDE.md` gains a short "Go version" section stating the single number and
  that there is no separate dev pin. `CLAUDE.md` is Dev-owned, so this edit
  belongs to whoever does the work.
- `README.md` states the version the repo is built and tested at. Not phrased
  as a "consumer minimum" — nothing is published (see above).
- CI pins the same 1.23 (`ai-context/tasks/github-actions-ci.md`). Whichever of
  the two lands second matches the other; neither blocks the other.

**The trap.** The `go` directive gates *language features*, not just the
toolchain that runs. Lowering it can turn code that compiles today into a
compile error. So every module must build, `go vet`, and `go test` clean at
1.23 before this is done — a green `core` proves nothing about `examples`. If
a module genuinely needs a post-1.23 feature, raise **that module's** directive
and record which feature forced it. Do not revert the project-wide decision
over one module.

**Out of scope**
- `golangci-lint`, dependency upgrades, and anything that changes what the
  modules require. This task moves one number and nothing else.

**Done looks like:** all seven files say `go 1.23`; `/verify` green; `CLAUDE.md`
and `README.md` state the number; **ADR 0014** written recording the ruling and
the rejected alternative (separate consumer and dev versions), the measured
per-module floors, and the `go.work` measurement artifact. The ADR is written
when the change is applied, not before — an ADR for an unapplied decision is a
plan, not a record.

## Test scenarios

**Black-box (tester, Gate 2):** `e2e/k6/run-sandbox.sh redis` and
`e2e/k6/run-sandbox.sh postgres` must both exit 0 with the existing contract
unchanged (5 accepted, 3 rejected in the burst scenario; the refill scenario's
checks all pass). The point is a compiled-and-running proof that lowering the
language version broke no behavior; no new scenario is needed.

**Unit / integration (Dev):** no new tests. The existing suite is the check —
it must pass at 1.23 in every module, and Dev reports the per-module result
rather than only the aggregate, since this task's whole risk is one module
failing where the others pass.
