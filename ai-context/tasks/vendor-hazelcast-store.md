# New backend — Hazelcast

**Goal (as proposed):** A `hazelcaststore` module implementing `core.CASStore`.

## Recommendation: do not build

This is scoped below to the same depth as `etcdstore`/`mongostore` so the
decision can be overridden with a real spec in hand, not a guess — but the
honest answer is **no fifth backend right now.** Two independent reasons, either
one sufficient on its own:

1. **It repeats a known defect on day one.** `ai-context/tasks/next-roadmap.md`
   row 8 (tier 1, owner-ruled) is that all four existing backends fail to
   forward `core.ProxyOption` to their own `Option` type, making
   `core.WithClock` unreachable and forcing wall-clock sleeps in expiry tests.
   A new backend built against the current pattern inherits that defect
   immediately; it should be fixed once, upstream of any new backend, not
   copied a fifth time.
2. **No new teaching value.** The portfolio's job (`ai-context/README.md`) is
   proving the provider-swap guarantee across genuinely different concurrency
   primitives: CAS-on-bytes (Redis, etcd, MongoDB) and exclusive-lock
   (Postgres). Hazelcast's `ReplaceIfSame` is a fourth CAS-on-bytes
   implementation of a pattern already proven three times over, with the same
   expire-on-read TTL story as Redis/etcd (see classification below) — it adds
   JVM container weight to CI (heaviest of any backend by a wide margin, see
   §4) and one more module to maintain, for zero new correctness lesson.

If the owner wants breadth over depth anyway, the scope below is ready to hand
to Dev as-is.

**Independence (if built):** Self-contained. No change to `core`, `redisstore`,
`pgstore`, `etcdstore`, or `mongostore`.

## Scope (ratified — implement exactly this, if this is picked up)

### 1. CAS or Lock — confirmed CASStore

The maintained Go client, `github.com/hazelcast/hazelcast-go-client`
(verified at pkg.go.dev, v1.6.0, `proxy_map.go:1245`), exposes:

```go
func (m *Map) ReplaceIfSame(ctx context.Context, key, oldValue, newValue interface{}) (bool, error)
```

present since v1.0.0 of the Go client — not a Java-only primitive. `false,
nil` means the compare failed (lost race), matching `core.CASStore`'s
`swapped=false, err=nil` contract exactly. Also available and needed:
`Get(ctx, key) (interface{}, error)`, `PutIfAbsent(ctx, key, value)
(interface{}, error)` for create-only-if-absent, `SetWithTTL(ctx, key, value,
ttl time.Duration) error`, and `Remove(ctx, key) (interface{}, error)`.

**Byte-equality risk — the part that can quietly fail.** `core.CASStore`
needs byte-for-byte equality on an opaque `[]byte` blob (`core/codec.go`
produces a deterministic 25-byte encoding specifically so this holds). Hazelcast
IMap's default in-memory format (`BINARY`) stores each value as the
client-serialized `Data` byte array, and `replace`/equality checks compare
that serialized form — not a deserialized-then-`Equals` comparison. For a raw
`[]byte` value the Go client's default serializer produces a deterministic
length-prefixed encoding (the same bytes in, same bytes out), so byte equality
should hold. This is the one claim in this file **not independently
byte-verified against a running cluster** — everything else here is checked
against the client's godoc; this one is inferred from Hazelcast's documented
BINARY in-memory format plus the Go client's default `[]byte` serializer.
**Dev must pin this with an integration test before writing anything else**:
write a state blob, mutate a copy, call `ReplaceIfSame` with the original as
`oldValue`, assert it succeeds; then assert a byte-flipped `oldValue` fails.
If that test does not hold, stop and report back rather than working around
it — a CAS store whose "byte equality" is actually Java-side `equals()` or a
hash comparison is not a fit for `core.CASStore` and should not be forced into
one.

### 2. ADR-0012 classification: same side as Redis/etcd (TTL is authoritative)

Hazelcast's expiration check applies uniformly at the map-operation layer, not
only at `Get`: an entry past its TTL is treated as absent by every operation
(`Get`, `PutIfAbsent`, `ReplaceIfSame`) even in the window before the
background expiration task physically evicts it. That is the same shape as
Redis (lazy expiration checked on access) and etcd (lease-based, server
deletes and reports absent) — no application-level `expiresAt` field, no
extra "absent-or-expired" predicate on writes, unlike `pgstore`/`mongostore`.
Confirm this specifically for `PutIfAbsent` against an expired-but-not-yet-evicted
entry with a test before relying on it (same caution as above); if it turns
out `PutIfAbsent` does *not* treat expired-as-absent, this backend moves to
the `pgstore`/`mongostore` side of ADR-0012 and needs its own
`expiresAt`-style filtering, which changes the value shape in §3.

### 3. Deviations from the etcd/mongo pattern

- **`ProxyOption` forwarding:** do not repeat the existing defect. Wire
  `core.ProxyOption` through from day one — this backend should be the first
  one that gets it right, whichever lands first between this task and row 8.
- **Key naming:** mirror `redisstore`/`etcdstore`'s `WithKeyPrefix(string)`
  (default empty); Hazelcast's `Map` is already a namespace (one map per
  logical bucket keyspace), same role as Mongo's collection, so no separate
  prefix is required for correctness — offer it only for parity with the
  other CAS backends' options.
- **Value shape:** the raw `core/codec.go` blob stored directly as the Map
  value (`[]byte` in, `[]byte`/`interface{}` out) — no wrapper struct, no
  version field, matching etcd's "arbitrary bytes" treatment. No BSON-style
  document is needed since ADR-0012 puts this backend on the no-`expiresAt`
  side (pending confirmation above).
- **`SchemaSQL` equivalent:** none — no setup step, matching etcd.
- **`NewProxyManager` signature:** takes a `*hazelcast.Client` (or a `*Map`
  directly, decide against whichever gives symmetric lifetime ownership with
  `redis.Cmdable`/`*pgxpool.Pool`/`*mongo.Collection` — likely a `*Map` for
  the same reason `mongostore` takes a `*mongo.Collection` and not a
  `*mongo.Client`).

### 4. Testcontainers image and CI weight

No official `testcontainers-go/modules/hazelcast` exists (checked against the
current module list on `testcontainers/testcontainers-go`; etcd has the same
gap and used `testcontainers.GenericContainer`, same approach here). Image:
`hazelcast/hazelcast:5-slim` (~134 MB compressed vs. `hazelcast/hazelcast:5`'s
~618 MB — use the slim tag). **This is a JVM member**, not a single process:
cold start plus cluster-ready takes materially longer than Redis, etcd, or
Postgres (seconds, not milliseconds) — wait on the "is STARTED" log line, not
just the port. A GitHub Actions workflow is landing on a sibling branch right
now (`ai-context/tasks/next-roadmap.md` row 2); a fifth container in that
matrix, and the heaviest one by both image size and JVM boot time, is a real
CI-minutes cost that should be weighed against §"Recommendation" before
merging this task's scope into that workflow.

### 5. Cost/value

Covered under "Recommendation" above — do not restate.

## Test scenarios (if built)

**Black-box (tester, `e2e/k6/run-sandbox.sh hazelcast`):** same sandbox
scenario already run for `redis`/`postgres`/`etcd`/`mongodb` — burst up to
capacity accepted, next request rejected with 429, request after the
advertised wait accepted again. No new black-box behavior versus the existing
backends; this backend does not change what a client observes, only what
stores the bytes.

**Unit/integration (Dev), against a real container per
`docs/adr/0006-testcontainers-for-backend-tck.md`:**
- `core/tck.Run` passes end to end (this is the provider-swap proof, AC-46).
- The byte-equality pin from §1: `ReplaceIfSame` succeeds against the exact
  original blob and fails (not errors) against a mutated one.
- Create-only-if-absent (`old == nil`) via `PutIfAbsent` succeeds once and a
  second call for the same key returns "not swapped", not an error.
- The ADR-0012 pin from §2: write a value with a short TTL, wait past
  expiry, then attempt `PutIfAbsent` for the same key — must succeed
  (treated as absent), not fail as "already present". If it fails, that is
  the signal to reclassify per §2, not to add a workaround silently.
- `Remove` on an absent key is not an error.
- `ProxyOption` (e.g. `core.WithClock`) actually reaches the store's TTL
  computation — the regression test row 8 is asking every backend to have.

**Status:** Scoped — not recommended for build. If the owner overrides the
recommendation, this file is ready to hand to Dev as-is; the two "Dev must
pin this first" checks in §1 and §2 gate everything else in the task.
