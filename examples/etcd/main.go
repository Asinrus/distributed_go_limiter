// Command etcd-example rate-limits a net/http handler with the etcd backend.
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
	"strings"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/asinrus/distributed_go_limiter/etcdstore"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func main() {
	endpoints := os.Getenv("ETCD_ENDPOINTS")
	if endpoints == "" {
		endpoints = "localhost:2379"
	}
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   strings.Split(endpoints, ","),
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Fatalf("etcd connect: %v", err)
	}
	defer cli.Close()

	pm := etcdstore.NewProxyManager(cli, etcdstore.WithTTL(time.Hour))

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
