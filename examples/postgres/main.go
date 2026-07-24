// Command postgres-example is the smallest useful demonstration of the
// distributed limiter's Postgres backend: build a *core.ProxyManager over a
// pgxpool.Pool, then call it exactly like any Bucket — TryConsume, nothing
// else — from an HTTP handler.
//
//	docker compose up -d
//	go run .
//	curl localhost:8080          # 200 five times, then 429
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/asinrus/distributed_go_limiter/pgstore"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()

	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		dsn = "postgres://limiter:limiter@localhost:5432/limiter?sslmode=disable" // matches docker-compose.yml
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	defer pool.Close()

	// Creates the bucket table if it does not already exist. Safe to call on
	// every boot.
	if err := pgstore.EnsureSchema(ctx, pool); err != nil {
		log.Fatalf("postgres schema: %v", err)
	}

	// One ProxyManager for the process. Postgres is the only backend-specific
	// line here: everything after this point is the same core.ProxyManager API
	// as the Redis example, or the in-memory one, or LocalBucket.
	//
	// WithTTL marks state as expired after this long, so a stale bucket reads as
	// a fresh one. Unlike Redis, Postgres never deletes the row on its own — a
	// real deployment sweeps expired rows on a timer with pgstore.Store.RemoveExpired,
	// left out here to keep this example to the one thing it demonstrates.
	pm := pgstore.NewProxyManager(pool, pgstore.WithTTL(time.Hour))

	// Deliberately slow — one token back every 12s — so the limit is reachable by
	// curling six times in a row. A realistic 5/second refills faster than you
	// can type, and a working limiter looks broken.
	limit := core.NewConfig(core.NewBandwidth(5).RefillGreedy(5, time.Minute))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// The state for this key lives in Postgres, not in this process — run a
		// second copy of this program against the same database and both draw
		// from the SAME bucket. That sharing is the entire point of the library;
		// an in-process limiter (e.g. golang.org/x/time/rate) cannot do it.
		b, err := pm.Bucket(clientKey(r), limit)
		if err != nil {
			// Only an invalid Config reaches here, and Config is fixed above, so
			// this fires on every request after a bad deploy, not intermittently.
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		ok, err := b.TryConsume(r.Context(), 1)
		if err != nil {
			// Postgres is unreachable. Failing closed (503) protects whatever is
			// behind this handler; failing open (serve anyway) protects
			// availability instead. There is no default that is right for every
			// endpoint — this example picks closed.
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
		listenAddr = ":" + p // lets two replicas run side by side, sharing one Postgres
	}
	log.Println("listening on " + listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, nil))
}

// clientKey strips the port from RemoteAddr. Every TCP connection gets a
// different ephemeral port, so keying on RemoteAddr as-is would put every
// request in its own bucket — the limit would never trigger.
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr // no port present; use as-is
	}
	return host
}
