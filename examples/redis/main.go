// Command redis-example serves the shared API (see ../api) rate-limited through
// Redis. Everything specific to Redis is in this file; the router, middleware and
// handlers are identical to the Postgres example.
//
//	docker compose up -d
//	go run .
package main

import (
	"log"
	"os"
	"time"

	redisext "github.com/asinrus/distributed_go_limiter/redis"
	"github.com/redis/go-redis/v9"

	"github.com/asinrus/distributed_go_limiter/examples/api"
)

func main() {
	ctx, stop := api.SignalContext()
	defer stop()

	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379" // matches docker-compose.yml
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: addr,

		// Timeouts are not optional for a limiter in the request path: every
		// call sits between your user and your handler. Better to fail the
		// limiter fast and take the fail-open/fail-closed branch than to let a
		// slow Redis add its latency to every request.
		DialTimeout:  2 * time.Second,
		ReadTimeout:  500 * time.Millisecond,
		WriteTimeout: 500 * time.Millisecond,

		// One round trip per rate-limit check, so size the pool for concurrent
		// in-flight requests, not for total clients.
		PoolSize:     20,
		MinIdleConns: 5,
	})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis %s unreachable: %v (is `docker compose up -d` running?)", addr, err)
	}

	pm := redisext.NewProxyManager(rdb,
		// Namespaces every key. Worth setting even on a dedicated instance: it
		// makes limiter keys obvious in redis-cli and lets one Redis serve
		// several apps or environments.
		redisext.WithKeyPrefix("ratelimit:"),

		// Redis expires keys itself, so a TTL is all the cleanup you need — no
		// sweeper, unlike Postgres. Any TTL comfortably longer than the time to
		// refill a bucket is safe: a bucket that idle has refilled to full
		// anyway, so dropping it changes nothing.
		redisext.WithTTL(time.Hour),
	)

	// rdb is a redis.Cmdable, so a cluster or Sentinel client works unchanged:
	//
	//	rdb := redis.NewClusterClient(&redis.ClusterOptions{Addrs: []string{...}})
	//	rdb := redis.NewFailoverClient(&redis.FailoverOptions{MasterName: "mymaster", ...})
	//
	// On Cluster, one bucket is a single key, so every operation is
	// single-slot — no cross-slot errors, no hash tags needed.

	api.Serve(ctx, api.Router(pm, "Redis ("+addr+")"))
}
