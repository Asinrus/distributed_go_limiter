# Examples

Two folders, each ~90 lines, each doing one thing: build a `*core.ProxyManager`
for a backend, then call it — `pm.Bucket(key, cfg)`, `TryConsume` — from a plain
`net/http` handler. That is the whole interface. Nothing else is in these files.

```
redis/        main.go + docker-compose.yml
postgres/     main.go + docker-compose.yml
```

## Run one

```bash
export PATH="$HOME/.local/go/bin:$PATH" GOTOOLCHAIN=local

cd examples/redis      # or examples/postgres
docker compose up -d
go run .
```

```console
$ for i in $(seq 6); do curl -s -o /dev/null -w '%{http_code} ' localhost:8080/; done
200 200 200 200 200 429
```

5 requests per minute, burst 5, keyed by client IP. Deliberately slow — one token
back every 12s — so the limit is reachable by hand. A realistic 5/second refills
faster than you can type curl, and a working limiter looks broken.

## The property that matters: two processes, one budget

```bash
docker compose up -d
PORT=8080 go run . &
PORT=8081 go run . &

for p in 8080 8081 8080 8081 8080 8081; do
  curl -s -o /dev/null -w "$p -> %{http_code}\n" localhost:$p/
done
```

```
8080 -> 200
8081 -> 200
8080 -> 200
8081 -> 200
8080 -> 200
8081 -> 429      <-- second process, same budget, the 6th request over the limit
```

Verified identical on both backends: alternating requests across two live,
independently-running processes drain **one** shared bucket down to exactly
zero, and the 6th request — landing on whichever process is up — is the one
that's rejected. `golang.org/x/time/rate`, or a `map[string]*rate.Limiter`
kept in-process, cannot do this: each process would keep its own count, and
doubling the replicas would silently double the effective limit.

## What each file actually shows

- Building a `*core.ProxyManager`: `redisstore.NewProxyManager(rdb, ...)` /
  `pgstore.NewProxyManager(pool, ...)` — the one line that's backend-specific.
- `pm.Bucket(key, cfg)` returning `(Bucket, error)` — config is validated once,
  here, not on every call.
- `b.TryConsume(ctx, 1)` — the actual rate-limit check, and the three ways it can
  come back: allowed, rejected (429), or the store is unreachable (503, a
  fail-closed choice made explicit in a comment — fail-open is equally
  defensible and endpoint-dependent).

## Traps worth knowing

- **`r.RemoteAddr` includes a port**, and every TCP connection gets a different
  ephemeral one. Both examples strip it (`clientKey`) — keying on the raw value
  would put every single request in its own bucket, and the limit would never
  trigger. This is not a hypothetical: it broke the first version of this
  example, silently, with every request returning 200.
- **`WithTTL` matters**, or every client you ever rate-limited stays in the store
  forever. Redis expires the key itself; Postgres does not — a real deployment
  sweeps expired rows on a timer (`pgstore.Store.RemoveExpired`), left out here to
  keep the example to the one thing it demonstrates.

## No container at all

```go
pm := core.NewInMemoryProxyManager()
```

Same `ProxyManager`, same command executor, same wire codec as the real
backends — a map instead of a store. Fine for local dev and tests; the two
processes above would NOT share a limit with each other on this one, since
there is nothing between them.
