# Changelog

The five library modules (`core`, `redisstore`, `pgstore`, `mongostore`,
`etcdstore`) all start at `v0.1.0` and are versioned independently from there
— each is tagged when its own code changes, and an entry names the module and
version it lands in when that is not obvious. `examples` is not tagged. See
`docs/adr/0021-modules-version-independently-after-v0-1-0.md`.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning:
[Semantic Versioning](https://semver.org/).

## Unreleased

## 0.1.0 - 2026-10-15

First release. This repository stays private at this tag — the artefacts are
tagged and verifiable by the owner, but not yet fetchable or indexed anywhere
public.

### Added

- `core.Bucket`: a token-bucket rate limiter with one bandwidth per bucket,
  greedy or intervally refill, `TryConsume`, `EstimateAbilityToConsume`, and
  a reservation-based `BlockingBucket.Consume` that waits without busy-spinning.
- Four storage backends behind the same `core.Bucket` API — `redisstore`
  (Lua compare-and-swap), `pgstore` (`SELECT ... FOR UPDATE`), `mongostore`
  (conditional `ReplaceOne`), and `etcdstore` (`Txn` + lease-backed expiry) —
  swappable by changing only the `*core.ProxyManager` constructor.
- A shared conformance suite (`core/storetest`) that every backend runs
  against a real container of its vendor; this is the proof that swapping
  providers does not change rate-limiting behaviour.
- Apache-2.0 licensing across the repo root and every module root.

### Changed

- Two points where this rewrite deliberately decides differently from
  bucket4j, recorded in
  `docs/adr/0018-deliberate-deviations-from-bucket4j.md`: reads never expose
  a negative token count, and a cancelled blocking reservation is refunded.

### Known limitations

- The byte layout `core/codec.go` persists is a private wire format between
  this library and itself, not a public interface, and will change before
  v1.0 (multi-bandwidth needs more than the one state it stores today). Do
  not parse the stored bytes or expect them to survive a format change
  across an upgrade.
