// Command postgres-example serves the shared API (see ../api) rate-limited
// through Postgres. Everything specific to Postgres is in this file; the router,
// middleware and handlers are identical to the Redis example.
//
//	docker compose up -d
//	go run .
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	pgext "github.com/asinrus/distributed_go_limiter/postgres"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/asinrus/distributed_go_limiter/examples/api"
)

const ttl = time.Hour

func main() {
	ctx, stop := api.SignalContext()
	defer stop()

	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		dsn = "postgres://limiter:limiter@localhost:5432/limiter?sslmode=disable" // matches docker-compose.yml
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Fatalf("bad PG_DSN: %v", err)
	}

	// Pool sizing is the Postgres-specific concern. Every rate-limit check opens a
	// transaction and holds a row lock (SELECT … FOR UPDATE) until it commits, so
	// a connection is tied up for the whole check. Undersize this and the limiter
	// becomes the bottleneck it was meant to prevent.
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour

	// A stuck lock must not hold a connection forever. Postgres will abort a
	// statement that outlives this, releasing the row.
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "3000" // ms

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("postgres unreachable: %v (is `docker compose up -d` running?)", err)
	}

	// Creates the table and its expiry index if absent. Safe to call on every
	// boot; it refuses to touch a table whose key column is not TEXT rather than
	// silently leaving a stale schema in place. Run it once at deploy time if you
	// would rather not grant DDL to the app role.
	if err := pgext.EnsureSchema(ctx, pool); err != nil {
		log.Fatalf("postgres schema: %v", err)
	}

	// Build the store once and share it: the ProxyManager limits, and the sweeper
	// below reuses the same store. pgext.NewProxyManager(pool, …) is the shortcut
	// that does this for you when you do not need the store handle.
	store := pgext.NewStore(pool, pgext.WithTTL(ttl))
	pm := core.NewLockProxyManager(store, core.WithExpireAfterWrite(ttl))

	// THE operational difference from Redis: Postgres does not expire rows on its
	// own. Expired state already reads as a fresh bucket, so this is housekeeping
	// rather than correctness — but without it the table grows for every key you
	// ever limited. One replica running this is enough; SKIP LOCKED means several
	// can run it concurrently without fighting.
	go sweepExpired(ctx, store, 5*time.Minute)

	api.Serve(ctx, api.Router(pm, "Postgres"))
}

func sweepExpired(ctx context.Context, store *pgext.Store, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// Bounded batch: a DELETE over a huge backlog would hold locks and
			// bloat WAL. Loop on the next tick instead.
			n, err := store.RemoveExpired(ctx, 1000)
			if err != nil {
				log.Printf("sweep expired: %v", err) // transient; try again next tick
				continue
			}
			if n > 0 {
				log.Printf("swept %d expired bucket(s)", n)
			}
		}
	}
}
