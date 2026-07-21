# Examples

A small Gin API rate-limited by this library, wired to each backend.

```
api/          the HTTP layer — router, handlers, rate-limit middleware
redis/        main.go + docker-compose.yml   ← Redis configuration
postgres/     main.go + docker-compose.yml   ← Postgres configuration
```

`api/` is shared and contains **no provider-specific code**. `redis/main.go` and
`postgres/main.go` differ only in how they build the `*core.ProxyManager` they
hand to `api.Router`. That is the library's whole claim, so the layout is
arranged to make a regression obvious: if either program ever needs a
provider-specific branch inside `api/`, the swap goal has broken.

## Run one

```bash
export PATH="$HOME/.local/go/bin:$PATH" GOTOOLCHAIN=local

cd examples/redis      # or examples/postgres
docker compose up -d
go run .
```

```console
$ for i in $(seq 6); do curl -s -o /dev/null -w '%{http_code} ' localhost:8080/api/search; done
200 200 200 200 200 429
```

Both folders serve the identical API:

| Route | Limit | Cost |
|---|---|---|
| `GET /healthz` | none — a liveness probe must never spend tokens | — |
| `GET /api/search` | 5 / minute, burst 5 | 1 token |
| `GET /api/heavy` | 10 / minute, separate budget | 5 tokens |

Keyed by `X-API-Key` when present, otherwise by client IP.

The rate is deliberately slow (one token back every 12s) so a 429 is reachable by
hand. At a realistic 5/second a token returns every 200ms — faster than you can
type — and a working limiter looks broken.

## The part an in-process limiter cannot do

Two replicas, one budget:

```bash
docker compose up -d
PORT=8080 go run . &
PORT=8081 go run . &

for p in 8080 8081 8080 8081 8080 8081; do
  curl -s -D- -o /dev/null -H 'X-API-Key: demo' localhost:$p/api/search \
    | awk -v p=$p '/Ratelimit-Remaining/{print p" remaining="$2}'
done
```

```
8080 remaining=4
8081 remaining=3      <-- other replica, same budget
8080 remaining=2
8081 remaining=1
8080 remaining=0
8081 remaining=       <-- 429
```

Verified identical on both backends. `golang.org/x/time/rate` — and the
`map[string]*rate.Limiter` middleware in Gin's own security guide — is correct on
one box and wrong on two, and the map grows for every client that ever calls you.

## No container at all

`core.NewInMemoryProxyManager()` is a drop-in `*core.ProxyManager` using the same
command executor and wire codec as the real backends, with a map for a store.
Good for local dev and tests, useless across replicas:

```go
pm := core.NewInMemoryProxyManager()
api.Serve(ctx, api.Router(pm, "in-memory"))
```

## Traps worth copying out of these files

- **`SetTrustedProxies`** ([api/api.go](api/api.go)). Gin trusts every proxy by
  default, so `ClientIP()` believes any `X-Forwarded-For`. For an IP-keyed limiter
  that is a bypass — forge the header, mint a fresh bucket. Set `nil` to trust
  nobody, or list your load balancer's CIDR.
- **One scope per Config** ([api/limiter.go](api/limiter.go)). v1 does not store
  the Config with the state, so two limits with different Configs must never
  share a key: last writer wins and both become wrong.
- **Decide fail-open vs fail-closed.** When the store is unreachable the
  middleware returns `503` (fail closed). Serving the request instead is equally
  defensible. There is no safe default, and it can differ per endpoint.
- **Blocking consume is not for handlers.** `AsBlocking().Consume` waits for
  capacity — right for background workers and outbound API calls, wrong for a
  request thread you would be parking for 12s. See `core/blocking.go`.
