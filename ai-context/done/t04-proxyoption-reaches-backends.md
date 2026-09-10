# T04 — `core.ProxyOption` reaches every backend

**Blocked by:** T02. Both touch every backend's test file; sequencing them
avoids a rename and a signature change colliding in the same four files.

**For:** Dev. Roadmap row 8.

## The defect

Each backend's `NewProxyManager` takes its *own* `Option` type and forwards
none of `core`'s:

- `redisstore/redisstore.go:110`
- `pgstore/pgstore.go:254`
- `mongostore/mongostore.go:143`
- `etcdstore/etcdstore.go:190` (roadmap row 8 cites `:107` — stale, the file
  moved)

So `core.WithClock` (`core/proxymanager.go:31`), `core.WithExpireAfterWrite`
(`core/proxymanager.go:36`) and `core.WithMaxCASRetries`
(`core/proxymanager.go:52`) are all unreachable from a backend constructor.
This is a pluggability defect, not an ergonomics one: `core` grows an option,
every backend silently fails to expose it, and the "swap the backend, keep the
code" claim quietly stops holding for anything configured. It is tier 1 for
that reason, by the owner's ruling that cheap pluggability is the product.

Note that each backend already re-implements one of them — `WithTTL` on all
four (`redisstore/redisstore.go:105`, `pgstore/pgstore.go:44`,
`mongostore/mongostore.go:108`, `etcdstore/etcdstore.go:186`) duplicates
`core.WithExpireAfterWrite`, because the TTL has to reach the store as well as
the manager. Whatever shape you pick has to keep that working; a backend option
and a core option that both set a TTL and disagree is worse than the bug.

## In scope

- Every backend's `NewProxyManager` accepts `core.ProxyOption` in addition to
  its own `Option`, and passes them through to
  `core.NewCASProxyManager` / `core.NewLockProxyManager`. Pick one shape and
  use it in all four — a backend author copying an existing backend is the main
  audience here, so consistency matters more than which shape.
- Existing call sites keep compiling, or every one in the repo is updated
  (`examples/*/main.go` and the four test files). Say which in the change
  description.

## Out of scope — and one correction to the roadmap

**The proof named when this was scoped does not hold.** The claim was that
row 8 makes the etcd bucket-expiry test instant
(`etcdstore/etcdstore_test.go:245`, a 3-second sleep — the roadmap and
`ai-context/done/etcd-as-a-main-backend.md` both cite `:132`, which is stale
and is now the lease-count test). It does not, and neither half of it does:

- `core.WithClock` sets the **refill** clock — the `now` a `bucketState` refills
  against. Expiry is enforced somewhere else entirely.
- Redis expiry is `EXPIRE`, etcd expiry is a lease. Both are the vendor's
  clock, server-side, and no client option shortens them. etcd's floor is
  roughly 3x its election timeout on top of that.
- pgstore and mongostore *do* evaluate expiry client-side
  (`pgstore/pgstore.go:129`, `mongostore/mongostore.go:50`) against an
  unexported `now` field. Making **that** clock injectable would speed up their
  expiry tests — but it is a store-level clock, a different thing from
  `core.WithClock`, and it is **not this ticket**. If it is worth doing, it is
  its own row.

**What forwarding `WithClock` actually buys** is the reason this ticket blocks
T05: every conformance case today has to use `slowConfig`
(`core/storetest`'s `slowConfig`, 1 token/hour) precisely so that refill *never* happens
during a test. Nothing involving elapsed time — a refill wait, a probe's
`NanosToWaitForRefill`, a blocking `Consume` — can be asserted against a real
backend without either an injectable clock or a real sleep. T05 needs the
first.

## Test scenarios

**Black-box (tester, Gate 2):** no new scenario — the examples call the
backend constructors but pass no core option, and the observable HTTP contract
is unchanged. Run `e2e/k6/run-sandbox.sh redis` **and** `postgres` as a
no-regression check that the constructor change did not break the demo wiring
(5 accepted, 3 rejected; refill checks pass; exit 0). Two backends here rather
than one because this ticket edits every constructor.

**Unit / integration (Dev):** one case per backend, plus the collision case.

- Per backend (`redisstore`, `pgstore`, `mongostore`, `etcdstore`): construct a
  manager with an injected `core.WithClock` against the real container, consume
  the bucket to empty, advance the injected clock past a refill period, and
  assert `AvailableTokens` reports the refilled count **with no wall-clock
  sleep**. This is the case that proves the option arrives, and it is the
  capability T05 builds on.
- Per backend: `core.WithMaxCASRetries` is reachable and takes effect —
  an out-of-range value is rejected by `Bucket` exactly as it is on the
  in-memory manager. Cheap, and it proves the plumbing is generic rather than
  special-cased for the clock.
- **TTL collision:** for each backend, a manager built with both the backend's
  `WithTTL` and `core.WithExpireAfterWrite` behaves predictably — assert the
  documented precedence rather than whichever the implementation happens to
  pick. Whatever you decide, state it in the constructor's godoc; a caller who
  sets both and gets the other one is a silent correctness bug in a rate
  limiter.
- Existing per-backend suites (AC-41 … AC-44 and each backend's own tests) pass
  unchanged.

**Done looks like:** a backend author can reach every `core.ProxyOption` from
every backend constructor, the four constructors take options the same way, and
a refill can be driven by an injected clock against a real container.
