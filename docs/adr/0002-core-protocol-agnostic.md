# `core` stays protocol-agnostic

An `net/http` middleware for `core` was built, then deleted. `core` now has no
transport code of any kind — no HTTP, no gRPC — and store failures surface
only as `core.ErrStore`, leaving fail-open vs fail-closed to the caller.
Baking in a transport meant `core` also had to pick a status code and a
degradation policy, decisions that belong to the application embedding the
limiter, not the library. An adapter belongs in the caller; `examples/`
demonstrates one per backend without `core` depending on it.

## Consequences

Any HTTP/gRPC integration work is example/adapter code, never a `core`
dependency. A caller distinguishes store failure from a legitimate reject with
`errors.Is(err, core.ErrStore)`.

Narrowed by `docs/adr/0015-cancellation-is-not-a-store-failure.md`: a cancelled
or expired context returns the bare context error, outside `ErrStore`.
