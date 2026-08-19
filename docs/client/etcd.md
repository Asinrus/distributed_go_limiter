# etcd backend

```go
cli, _ := clientv3.New(clientv3.Config{Endpoints: []string{"localhost:2379"}})
pm := etcdstore.NewProxyManager(cli, etcdstore.WithTTL(time.Hour))
```

That is the entire etcd-specific surface. Everything after it — `pm.Bucket(key, cfg)`,
`b.TryConsume(ctx, 1)` — is the same `core.ProxyManager` API the Redis and Postgres backends
use, and the same one `core.NewInMemoryProxyManager()` gives you with no container at all.

## Options

| Option | Default | Effect |
|---|---|---|
| `WithKeyPrefix(p)` | `""` | Namespaces bucket keys (e.g. `"dgl:"`). |
| `WithTTL(d)` | `0` (no expiry) | Expire-after-write, via a per-write lease. |

## `WithTTL` is the whole cleanup story

etcd deletes the key itself when its lease expires, so unlike Postgres there is nothing to
sweep — see `docs/adr/0012-ttl-is-not-a-correctness-mechanism.md`. Every successful write
grants a fresh lease and attaches it to that write (never `KeepAlive`, which would make state
immortal); the lease displaced by an overwrite is left to expire on its own.

etcd lease TTLs are granted in whole seconds with a minimum of 1, so a `WithTTL` under a second
rounds up. The etcd server also enforces its own minimum lease TTL (roughly three times its
election timeout, ~3s on a default single-node cluster), so the effective floor on an idle
bucket's lifetime can be higher than what was requested.

Without a TTL, every client you ever rate-limited stays in etcd forever.

## What is stored

25 bytes per bucket — a version byte plus three big-endian int64s. The token maths runs in
`core`, never in etcd; etcd provides only atomic get and a compare-and-put (`Txn`).

```console
$ etcdctl get --prefix ""
127.0.0.1
<25 raw bytes>
$ etcdctl lease timetolive $(etcdctl get 127.0.0.1 -w json | jq -r '.kvs[0].lease')
```

## Errors

Any etcd failure — I/O, timeout, cancellation — comes back wrapping `core.ErrStore`, with the
`clientv3` error still reachable underneath via `errors.Is`/`errors.As`. `core` does not pick
fail-open or fail-closed for you:

```go
ok, err := b.TryConsume(ctx, 1)
if errors.Is(err, core.ErrStore) {
    // your call: serve anyway (fail open) or reject (fail closed)
}
```
