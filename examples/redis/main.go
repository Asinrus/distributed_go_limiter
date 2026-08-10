// Command redis-example rate-limits a net/http handler with the Redis backend.
//
//	docker compose up -d
//	go run .                     # PORT=8081 go run . for a second, concurrent replica
//	curl localhost:8080          # 200 five times, then 429
package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/asinrus/distributed_go_limiter/redisstore"
	"github.com/redis/go-redis/v9"
)

func main() {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})

	pm := redisstore.NewProxyManager(rdb, redisstore.WithTTL(time.Hour))

	// One token back every 12s, so the limit is reachable by hand.
	limit := core.NewConfig(core.NewBandwidth(5).RefillGreedy(5, time.Minute))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		b, err := pm.Bucket(clientKey(r), limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		ok, err := b.TryConsume(r.Context(), 1)
		if err != nil {
			// Store unreachable. Fail closed here; fail open is equally defensible
			// and endpoint-dependent.
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		if !ok {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		fmt.Fprintln(w, "ok")
	})

	listenAddr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		listenAddr = ":" + p
	}
	log.Println("listening on " + listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, nil))
}

// clientKey strips the port from RemoteAddr. Every TCP connection gets a
// different ephemeral port, so keying on RemoteAddr as-is would put every
// request in its own bucket and the limit would never trigger.
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
