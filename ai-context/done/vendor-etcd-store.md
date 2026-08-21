# New backend — etcd

**Goal:** An `etcdstore` module implementing `core.CASStore`.

**Independence:** Self-contained. No change to `core`, `redisstore`, or
`pgstore`. Can be picked up in any order relative to the other vendor tasks
in this folder.

## Scope (ratified — implement exactly this)

- **Module/package:** `etcdstore`, module path
  `github.com/asinrus/distributed_go_limiter/etcdstore`, with
  `replace github.com/asinrus/distributed_go_limiter/core => ../core` and an
  entry in `go.work`. Mirror `redisstore`'s file layout.
- **Interface:** `core.CASStore`. Confirmed over `LockStore`: etcd's concurrency
  package does offer a distributed mutex, but it costs a lease plus a session
  per bucket operation to emulate a primitive `Txn` already gives us for free.
- **Client:** `go.etcd.io/etcd/client/v3`. `NewProxyManager(cli *clientv3.Client,
  opts ...Option)` — the caller owns the client's lifetime, as with `redis.Cmdable`
  and `*pgxpool.Pool`.
- **Key/value shape:** key is `prefix + bucketKey` (`WithKeyPrefix`, default
  empty — same option as `redisstore`). Value is the raw state blob; etcd values
  are arbitrary bytes, so no encoding wrapper and no version field.
- **Options:** `WithKeyPrefix(string)`, `WithTTL(time.Duration)`.

### CAS mechanics

Use `clientv3.Compare(clientv3.Value(k), "=", string(old))` — **not**
`ModRevision`. `core.CASStore` passes the previous *bytes*, never a revision
handle, so a ModRevision compare would require threading a version through the
contract; `core` deliberately has no such field, and etcd compares values
natively. `Txn.Commit()`'s `resp.Succeeded` is the swapped flag; a failed compare
is `swapped=false, err=nil`, never an error.

- `GetState`: `cli.Get(ctx, k)`; zero `Kvs` → `found=false, err=nil`.
- `CompareAndSwap` with `old == nil`:
  `Txn(ctx).If(clientv3.Compare(clientv3.CreateRevision(k), "=", 0)).
  Then(clientv3.OpPut(k, string(newState), putOpts...)).Commit()`.
- `CompareAndSwap` with `old != nil`:
  `Txn(ctx).If(clientv3.Compare(clientv3.Value(k), "=", string(old))).
  Then(clientv3.OpPut(k, string(newState), putOpts...)).Commit()`.
- `Remove`: `cli.Delete(ctx, k)`; deleting an absent key is not an error.

### TTL mechanics

Leases, and they are authoritative — etcd removes the key itself when the lease
expires, so unlike `pgstore`/`mongostore` there is **no application-level expiry
check anywhere in this backend**. See
`docs/adr/0012-ttl-is-not-a-correctness-mechanism.md`.

- For `ttl > 0`, `cli.Grant(ctx, seconds)` per write and pass
  `clientv3.WithLease(resp.ID)` on the `OpPut`. This is expire-after-write:
  each successful write attaches a fresh lease. Never call `KeepAlive` — that
  would make state immortal.
- etcd lease TTL granularity is whole seconds with a minimum of 1: round
  sub-second TTLs **up** to 1s, the same shape as `redisstore`'s PX rounding.
  Say so in the `WithTTL` godoc — a caller asking for 200ms gets 1s.
- Do not revoke the lease displaced by an overwrite; it detaches from the key
  and expires on its own. Leave a comment naming the ceiling: one `Grant` RPC
  per write, and reuse a shared lease per TTL if that cost ever shows up.
- `ttl <= 0` → no lease, no expiry.

**`SchemaSQL` equivalent: none.** etcd needs no setup; do not invent an
`EnsureSchema` for symmetry.

### Tests

`tck.Run` against a real container per
`docs/adr/0006-testcontainers-for-backend-tck.md`. testcontainers-go v0.33.0
(the version the other backends pin) has no etcd module — use
`testcontainers.GenericContainer` with the fully-qualified image
`quay.io/coreos/etcd:v3.5.16` (fully qualified because the local
`registries.conf` only searches docker.io), `--listen-client-urls
http://0.0.0.0:2379 --advertise-client-urls http://0.0.0.0:2379`, exposed port
2379, waiting on the port. `ctr.Terminate(ctx)`, no `TerminateContainer` helper.
Keep the `testing.Short()` skip that `redisstore` uses.

**Acceptance:** `core/tck` suite passes against a real etcd container. New
module in `go.work`. `examples/etcd/` demo mirroring the existing two (only the
`*core.ProxyManager` constructor differs), with its own `docker-compose.yml`,
plus `docs/client/etcd.md`.

**Status:** Scoped — ready for Dev.
