# Postgres backend

```go
pool, _ := pgxpool.New(ctx, dsn)
pm := pgstore.NewProxyManager(pool, pgstore.WithTTL(time.Hour))
```

That is the entire Postgres-specific surface. Everything after it — `pm.Bucket(key, cfg)`,
`b.TryConsume(ctx, 1)` — is the same `core.ProxyManager` API the Redis backend uses, and the
same one `core.NewInMemoryProxyManager()` gives you with no container at all.

## Options

| Option | Default | Effect |
|---|---|---|
| `WithTable(t)` | `bucket` | Table name. |
| `WithIDColumn(c)` | `id` | Primary-key column (`TEXT`). |
| `WithStateColumn(c)` | `state` | State column (`BYTEA`). |
| `WithTTL(d)` | `0` (no expiry) | Marks state stale after this long. Sugar for `WithProxyOption(core.WithExpireAfterWrite(d))`. See cleanup below. |
| `WithProxyOption(o)` | — | Forwards a `core.ProxyOption` (`core.WithClock`, `core.WithMaxCASRetries`, `core.WithExpireAfterWrite`, …) to the underlying `core.ProxyManager`. |

Identifiers cannot be bind parameters, so these names are interpolated into SQL — quoted and
escaped, never executed. That makes them case-sensitive and exact: `WithTable("Bucket")`
means the table `"Bucket"`, not `bucket`. It is also a single identifier, not schema-qualifiable:
`WithTable("myschema.bucket")` names a table literally called `myschema.bucket`, not `bucket`
inside schema `myschema`. To target a non-default schema, set `search_path` on the connection
and pass a bare table name.

## Schema

One row per bucket:

```
   Column   |  Type  | Nullable
------------+--------+----------
 id         | text   | not null
 state      | bytea  |
 expires_at | bigint |
Indexes:
    "bucket_pkey" PRIMARY KEY, btree (id)
    "bucket_expires_at_idx" btree (expires_at) WHERE expires_at IS NOT NULL
```

`state` is 25 bytes — a version byte plus three big-endian int64s. The token maths runs in
`core`; Postgres provides only the row lock (`SELECT … FOR UPDATE`).

### Applying it

Two supported paths. Pick one — they produce the same schema.

**1. Your own migration tool.** `SchemaSQL` returns the DDL as plain statements, in the order
they must run, so it can go into goose / Flyway / atlas / a `.sql` file. Generating it beats
copying the DDL into a migration by hand, because it honours `WithTable` / `WithIDColumn` /
`WithStateColumn` and does the identifier quoting for you.

```go
for _, stmt := range pgstore.SchemaSQL(pgstore.WithTable("rate_limit_bucket")) {
    fmt.Println(stmt + ";")
}
```

```sql
CREATE TABLE IF NOT EXISTS "rate_limit_bucket" ("id" TEXT PRIMARY KEY, "state" BYTEA, expires_at BIGINT);
CREATE INDEX IF NOT EXISTS "rate_limit_bucket_expires_at_idx" ON "rate_limit_bucket" (expires_at) WHERE expires_at IS NOT NULL;
```

Pass the *same* options to `NewProxyManager` afterwards, or the names will not match.

**2. `EnsureSchema` at boot.** Runs the same statements against a live pool. Idempotent, safe
to call on every start, and the right choice when the application owns its database.

```go
if err := pgstore.EnsureSchema(ctx, pool); err != nil { ... }
```

`EnsureSchema` does not *migrate*. Keys are `TEXT`; a table left over from an earlier
`BIGINT`-keyed build would survive `CREATE TABLE IF NOT EXISTS` untouched and then fail on
every query, so it is reported as an error instead:

```
pgstore: bucket.id is bigint, want text; drop and recreate the table
```

There is no upgrade path from that layout — a `BIGINT` key was a hash, and hash collisions
silently merged two unrelated clients into one shared limit. Drop the table and let it be
recreated; rate-limit state is disposable.

## Cleanup — Postgres does not expire rows

`WithTTL` marks state stale, so a bucket past its TTL reads as fresh and full. Nothing deletes
the row. Without a sweep, the table grows by one row for every client you ever limited:

```go
store := pgstore.NewStore(pool, pgstore.WithTTL(time.Hour))
for range time.Tick(time.Minute) {
    n, err := store.RemoveExpired(ctx, 1000)   // batched, FOR UPDATE SKIP LOCKED
    ...
}
```

`SKIP LOCKED` means the sweeper never blocks a live rate-limit check, and several replicas can
run it concurrently without fighting.

A read-only operation (`AvailableTokens`, `EstimateAbilityToConsume`) on a key that was never
written stores nothing, so footprint tracks writes, not reads — `RemoveExpired` has nothing to
reclaim from reads alone. A table written by a build predating AC-48 may still hold placeholder
rows (`state IS NULL`, no `expires_at`) leaked by reads; `DELETE FROM bucket WHERE state IS NULL;`
clears them.

## Server configuration

Every rate-limit check is a short transaction holding one row lock, so the workload is many
tiny transactions rather than few large ones. Give Postgres connection headroom to match the
application's `pgxpool` size, plus room for the sweeper and your own sessions.

The example's compose file also sets `fsync=off` and `synchronous_commit=off`. That is a
local-development trade of durability for write throughput. **Do not copy those to a shared
database.**

## Errors

Any Postgres failure — I/O, timeout, cancellation, lock contention past the retry limit —
comes back wrapping `core.ErrStore`, with the `pgx` error still reachable underneath via
`errors.Is`/`errors.As`. `core` does not pick fail-open or fail-closed for you:

```go
ok, err := b.TryConsume(ctx, 1)
if errors.Is(err, core.ErrStore) {
    // your call: serve anyway (fail open) or reject (fail closed)
}
```
