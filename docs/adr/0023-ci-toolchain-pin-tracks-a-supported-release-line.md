# The CI toolchain pin tracks a supported Go release line, not the `go` directive's

Amends `docs/adr/0017-one-go-directive-tracks-the-dependency-floor.md`. 0017's
ruling is **not** reopened: one `go` directive in every `go.mod` and `go.work`,
equal to the highest floor any module's dependencies force, no per-module
exception, no `toolchain` directive. All of that stands as written. The clause
this replaces is 0017's second sentence about CI — that the `setup-go` pin is
*"the newest patch of that minor line … so stdlib advisories on the line are
covered."*

The pin is instead the newest patch release of the **oldest Go minor line that
still receives security fixes** — the older of the two lines the Go project
supports — with `GOTOOLCHAIN=local` so it is enforced rather than silently
upgraded. Today that is `1.26.9` (`.github/workflows/ci.yml`, PR #41), against
a `go` directive of `1.25.0`.

The only remaining coupling between the two is a floor: the pin must be at
least the directive, because `GOTOOLCHAIN=local` turns a pin below any member's
directive into a hard build failure. Above that floor they are independent.

## Two knobs, named so they stop being confused

- The **`go` directive** — six `go.mod` files plus `go.work`. Consumer-facing:
  the toolchain a user of these modules must have. Derived from this repo's
  dependencies, and moves only when one of them forces it. 0017 owns it and
  nothing here changes it.
- The **CI toolchain pin** — one line of `.github/workflows/ci.yml`.
  Developer-facing: what builds, vets and tests, and what `govulncheck`
  compares the standard library against. Derived from Go's support window.
  This ADR owns it.

## Why the old clause could not hold

The Go project ships security fixes only to its two most recent major
releases. `1.25` left that window when `1.27.0` shipped, and `1.25.14` is the
last release it will ever get. GO-2026-6607 (`crypto/tls`, malformed ECH outer
extension references) records its fixed versions as `1.26.9` and `1.27.2` with
no `1.25` entry at all — so every `1.25.x` is affected permanently, not
pending a backport. Six sibling advisories in `crypto/tls` and `net/http` have
the same shape. They turned `main` red with no code change, in `mongostore`,
`redisstore` and `etcdstore`.

So the clause's closing promise — *"stdlib advisories on the line are
covered"* — was never a property of tracking the newest patch. It was a
property of the directive's line still being supported, which is a thing with
an expiry date that nobody in this repo controls. Written as a rule, it
guaranteed a future red pipeline.

Tying the pin to the directive's line leaves exactly three exits when the line
expires, and 0017's own reasoning rejects all three. Raise the directive to
`1.26`: 0017 rules the directive is derived from dependencies, not from CI
convenience, and no module's graph requires `1.26` — it would push every
v0.1.0 consumer up a minor line for a reason none of their dependencies has.
Stop scanning: `govulncheck` is the gate that caught this. Ship red: not a
decision, an abandonment.

## What was rejected

**Pin the newest stable release (`1.27.2` today).** Buys no coverage — both
supported lines carry every current stdlib fix, which is what "supported"
means — and pays for it with churn on a schedule this repo does not set, plus
a brand-new minor's own regressions landing in the only automated gate the
project has. The older supported line is the one already patched nine times.
0017 rejected chasing the latest release for the *directive*, on the different
ground that it moves the number above what anything requires; the conclusion
agrees, the reason does not transfer, and saying so is the point of naming the
two knobs separately.

**Float the patch** (`go-version: "1.26.x"`), letting `setup-go` resolve it per
run. Rejected on 0014's one surviving justification, which 0017 inherited: the
version local runs and CI agree on is the version a failure reproduces at. A
resolved-at-run-time pin is not a version named in the tree.

**Raise the `go` directive to `1.26.0` so the two share a line again.** This is
the instinct that produced the bug, so it gets named rather than left implicit.
Equal numbers were a coincidence that held while the floor's line was
supported, never a property worth paying for. The directive is a promise about
what a consumer needs; v0.1.0 is one commit and five tags away
(`docs/adr/0019`, `docs/adr/0021`) and nothing in the module graph asks for it.

**A vulnerability-exception file** so a known advisory could be waived in CI.
Not needed here: the fix exists, in a toolchain this repo is free to build
with. Worth revisiting only for an advisory with no fixed release at all,
which is a different problem than this one.

## Consequences

The pin's derivation now expires on a schedule: when `1.28.0` ships, `1.26`
leaves the support window and the pin becomes the newest `1.27` patch.
Deliberately, no scheduled scan or release-watch process is added to notice
that. The forcing function is the one that already fired — `govulncheck` in CI
goes red, in every module that links the affected stdlib packages — and this
ADR is the answer to "what do I set it to," not a reminder to look.

The pin may now sit a minor line or two above the directive, and routinely
will. That is sound: a newer toolchain builds a module declaring an older `go`
directive. It does **not** mean anything is tested at `go 1.25.0`; nothing is,
and no document should say it is. The directive states the floor a consumer's
toolchain must clear, which 0017 already accepted has no test matrix behind it
— if this repo ever needs one, that is 0017's named revisit, unchanged.
