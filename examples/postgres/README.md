# Postgres configuration

```bash
docker compose up -d
go run .                     # PORT=8081 go run . for a second, concurrent replica
docker compose down
```

Override the DSN with `PG_DSN` (default matches
[docker-compose.yml](docker-compose.yml):
`postgres://limiter:limiter@localhost:5432/limiter?sslmode=disable`).

## What [main.go](main.go) actually does

```go
pool, _ := pgxpool.New(ctx, dsn)
pgstore.EnsureSchema(ctx, pool)
pm := pgstore.NewProxyManager(pool, pgstore.WithTTL(time.Hour))
```

That's the entire Postgres-specific part. Everything after it — `pm.Bucket(key,
cfg)`, `b.TryConsume(ctx, 1)` — is the same `core.ProxyManager` API the Redis
example uses, and the same one `core.NewInMemoryProxyManager()` gives you with
no container at all.

**`EnsureSchema`** creates the bucket table if it does not already exist. Safe to
call on every boot; it refuses to touch a table whose key column is not `TEXT`
rather than silently leaving a stale schema in place — an earlier build of this
library hashed keys to `BIGINT`, where a collision would have silently merged two
unrelated clients into one shared limit.

**Unlike Redis, Postgres never expires a row on its own.** `WithTTL` marks state
as stale so it reads as a fresh bucket once past due, but nothing deletes the
row. A real deployment sweeps expired rows on a timer:

```go
n, _ := pgstore.NewStore(pool).RemoveExpired(ctx, 1000)  // batched, FOR UPDATE SKIP LOCKED
```

left out of `main.go` to keep this example to the one thing it demonstrates —
without it the table just grows for every client you ever limited.

## What is in the compose file, and why

**`fsync=off`, `synchronous_commit=off`** trade durability for write throughput.
Nothing here is worth recovering — `EnsureSchema` rebuilds the table on boot.
**Do not copy those two lines to a shared database.**

**`max_connections=100`** gives headroom over the small pool `pgxpool.New` opens
by default.

## How the state looks

One row per bucket: a TEXT key, 25 bytes of state, and an expiry.

```console
$ psql -U limiter -d limiter -c '\d bucket'
   Column   |  Type  | Nullable
------------+--------+----------
 id         | text   | not null
 state      | bytea  |
 expires_at | bigint |
Indexes:
    "bucket_pkey" PRIMARY KEY, btree (id)
    "bucket_expires_at_idx" btree (expires_at) WHERE expires_at IS NOT NULL

$ psql -U limiter -d limiter -c 'select id, length(state) from bucket;'
    id     | length
-----------+--------
 127.0.0.1 |     25
```
