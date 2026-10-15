# One `go` directive everywhere, tracking the highest dependency floor

**Superseded by `docs/adr/0024-go-directive-is-per-module.md`, in whole.** The
ruling this file exists to state — one number in every `go.mod` and `go.work`,
no per-module exception, no lower floor held by `core` — is reversed. Each
module now declares the floor its own dependency graph forces:
`golang.org/x/net v0.60.0`, the fix for four advisories reachable in
`etcdstore`, declares `go 1.26.0`, and nothing in `core`, `redisstore`,
`pgstore` or `mongostore` requires `x/net` at all. The clauses that survive
are restated in 0024 rather than left here to be hunted for — no `toolchain`
directive, the floor is derived and not chosen, chasing the newest Go release
stays rejected, and the `GOWORK=off` measurement method, which is now the
whole mechanism. Two of the premises below have also since gone:
`docs/adr/0021-modules-version-independently-after-v0-1-0.md` retired "what
version is this repo?" as a question a consumer asks, and
`docs/adr/0023-ci-toolchain-pin-tracks-a-supported-release-line.md` made the
CI pin an independent knob. Kept as the record of why one number was tried
and what it was answering.

Supersedes `docs/adr/0014-single-go-version.md`.

Every `go.mod` and `go.work` states the same `go` directive, and that number is
the **highest floor any module's own dependencies force** — today `go 1.25.0`.
There is no per-module exception, no lower "consumer" floor held by `core`, and
still no `toolchain` directive. CI's `setup-go` pin carries
`GOTOOLCHAIN=local`, so the pin is enforced rather than silently upgraded.

**Correction**, from
`docs/adr/0023-ci-toolchain-pin-tracks-a-supported-release-line.md`: the
sentence above originally set that pin at *"the newest patch of that minor
line (today `1.25.14`) … so stdlib advisories on the line are covered."* The
closing promise is not achievable. Go ships security fixes only to its two
newest major releases, so once the directive's minor line leaves that window
its newest patch is permanently vulnerable — GO-2026-6607 is fixed in `1.26.9`
and `1.27.2`, and never in `1.25`. The pin tracks the oldest Go line still
receiving security fixes, today `1.26.9`; it is chosen independently of the
`go` directive, subject only to being at least as new. Everything else in this
ADR — one directive, derived from the dependency floor, no per-module
exception, no `toolchain` directive — is unchanged by 0023.

## Why 0014's escape hatch had to go

0014 recorded an owner ruling — *one* version, not two — and then granted the
Architect an escape hatch: raise only the modules a dependency's security floor
forces. Within days the hatch had fired three times. `google.golang.org/grpc`
v1.82.1 (GO-2026-6061) declares `go 1.25.0`, forcing `etcdstore` and `examples`;
`github.com/jackc/pgx/v5` v5.9.2 (GO-2026-5004) declares `go 1.25.0`, forcing
`pgstore`. None of the raises was chosen and none needed a language feature —
they were the ordinary cost of staying non-vulnerable, which is a thing that
recurs by definition.

The result is that the repo reinstated exactly what the owner rejected: two
numbers to keep honest. The hatch merely split them along a different axis —
module instead of consumer-versus-developer. `core`, `redisstore`, and
`mongostore` at 1.23 against `pgstore`, `etcdstore`, `examples`, and `go.work`
at 1.25.0, plus CI at 1.25.14, is three numbers, and nobody could answer "what
version is this repo?" from the tree. 0014's own surviving justification — *the
version local runs and CI agree on is the version a failure reproduces at* —
argues against the split, not for it.

## What was rejected

**Keep the pin, retitle 0014 to "core holds a consumer-facing floor."** This is
the defensible-looking alternative and it fails on the repo's own stated purpose.
`CLAUDE.md` names the primary design goal as swapping the provider
(local ↔ Redis ↔ Postgres) without changing rate-limiting logic. A consumer
exercising that headline feature imports `pgstore`, whose module graph already
requires `go 1.25.0`. So the floor `core`'s directive appears to promise is not
available to anyone using the library as advertised — it protects only the
in-memory reference path. A promise that the flagship use case cannot keep is
worse than no promise, because it needs an ADR amendment per advisory to stay
even nominally true.

**Chase the latest Go release** (1.26.x at time of writing) rather than the
dependency floor. Rejected: it moves the directive above what anything requires,
which is churn bought with nothing. Tracking the floor makes the number derived
rather than chosen, so a reader can recompute it and a bump always has a named
cause.

## What this costs

A consumer on Go 1.23 or 1.24 can no longer import `core` — a module with zero
third-party runtime dependencies, whose own floor is nothing at all, now demands
a current toolchain for reasons entirely external to it. That is a real loss and
it is accepted deliberately: the owner has ruled this repo a portfolio /
reference implementation with no consumer base
(`ai-context/tasks/next-roadmap.md`), which is the same reason 0014 gave for
dropping the dev-versus-consumer split. Accepted with it: the directive now
moves whenever an advisory forces it, and each move is breaking for anyone
pinned lower. If this repo ever acquires real consumers, that is the trade-off
to revisit — and the fix then is a supported-version policy with a test matrix,
not a directive that quietly disagrees with the module graph.

## Measuring the floor

Per module, with `GOWORK=off`, from each module's own `go.mod` requirements —
the workspace unifies transitive versions across modules and hides each
module's real floor (0014 records an earlier reading that got 1.24 this way and
had to be reverted). Do not run `go mod tidy` to find out: inside this
workspace it rewrites the `go` directive upward instead of reporting a
conflict, so it answers the question by destroying the evidence. Read the
dependency's own `.mod` file instead.
