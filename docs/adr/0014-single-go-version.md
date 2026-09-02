# One Go version for development and consumers, no toolchain directive

The owner ruled that every `go.mod` and `go.work` in the repo states the same
`go` directive — one version, not a lower "consumer minimum" alongside a
higher version contributors actually build with. Rejected: pinning a
conservative floor for anyone importing the modules while CI and local
development run something newer. Two numbers to keep in sync bought nothing
here, and this repo is a portfolio / reference implementation
(`ai-context/tasks/next-roadmap.md`), not a near-term release with an
external consumer base to protect from a language-version bump. What survives
without that justification: the version local runs and CI agree on is the
version a failure can be reproduced at, and a repo whose stated purpose is to
demonstrate practice should run the practice it describes. No `toolchain`
directive — one pinned `go` line is the whole mechanism.

The number is derived, not chosen: measured per module with `GOWORK=off` (the
workspace unifies transitive versions across modules and hides each module's
own floor), counting only third-party runtime dependencies, the highest floor
any dependency forces is `etcdstore` at go1.22. `core` has zero third-party
runtime deps and forces nothing; `redisstore` floors at go1.18, `mongostore`
at go1.20, `pgstore` at go1.21. go1.23 clears all of them with margin and is
the most-used version in production Go today, so it is the number every
`go.mod` and `go.work` now states.

An earlier reading put the floor at go1.24. That reading measured inside the
workspace, where `go.work` unifies `golang.org/x/text` up to the v0.34.0 that
the `examples` module pulls in — a version none of the storage modules need on
their own. The correct measurement drops `GOWORK` and inspects each module in
isolation; re-deriving this number must do the same, not repeat the
in-workspace reading.

All seven files (`go.work`, `core`, `redisstore`, `pgstore`, `mongostore`,
`etcdstore`, `examples`) built, `go vet`'d, and `go test`'d clean at go1.23 —
no module needed a post-1.23 language feature, so no directive was raised
above the project-wide number.
