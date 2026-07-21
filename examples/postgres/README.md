# Postgres configuration

```bash
docker compose up -d
go run .            # PORT=8081 go run . for a second replica
docker compose down
```

Override the DSN with `PG_DSN` (default matches
[docker-compose.yml](docker-compose.yml):
`postgres://limiter:limiter@localhost:5432/limiter?sslmode=disable`).

## What is Postgres-specific here

Everything is in [main.go](main.go); the API in `../api` is untouched.

**You must sweep expired rows yourself.** This is the one real operational
difference from Redis, which expires keys on its own. Expired state already reads
as a fresh bucket, so this is housekeeping rather than correctness — but without
it the table grows for every key you ever limited:

```go
go sweepExpired(ctx, store, 5*time.Minute)   // store.RemoveExpired(ctx, 1000)
```

`RemoveExpired` deletes a bounded batch using `FOR UPDATE SKIP LOCKED`, so
several replicas can run it concurrently without fighting, and a huge backlog
never turns into one lock-holding, WAL-bloating `DELETE`. One replica running it
is enough.

**Pool sizing is the thing to get right.** Every check opens a transaction and
holds a row lock (`SELECT … FOR UPDATE`) until commit, so a connection is tied up
for the whole check. Undersize `MaxConns` and the limiter becomes the bottleneck
it was meant to prevent. Keep it under the server's `max_connections` with room
for the sweeper and your own `psql`.

**`statement_timeout`** stops a stuck lock holding a connection forever — Postgres
aborts the statement and releases the row.

**`EnsureSchema`** creates the table and its partial expiry index if absent. Safe
on every boot; it refuses to touch a table whose key column is not `TEXT` rather
than silently leaving a stale schema in place. Run it once at deploy time instead
if you would rather not grant DDL to the app role.

**Sharing one store** between the limiter and the sweeper is why this file calls
`pgext.NewStore` + `core.NewLockProxyManager` rather than the usual
`pgext.NewProxyManager(pool, …)` shortcut — which does exactly the same thing when
you do not need the store handle. It also shows the seam any new backend plugs
into.

Table and column names are configurable: `WithTable`, `WithIDColumn`,
`WithStateColumn`.

## What is in the compose file, and why

**`fsync=off`, `synchronous_commit=off`** trade durability for write throughput.
Nothing here is worth recovering — `EnsureSchema` rebuilds the table on boot.
**Do not copy those two lines to a shared database.**

**`max_connections=100`** gives headroom for the app pool plus the sweeper.

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
        id        | length
------------------+--------
 search:key:alice |     25
```

Keys are TEXT, not a hash. An earlier build hashed them to `BIGINT`, where an
FNV-64a collision would silently merge two unrelated clients into one shared
limit.
