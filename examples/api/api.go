// Package api is the HTTP layer shared by every backend example.
//
// Nothing in this package is provider-specific, and that is the point: the
// redis/ and postgres/ programs differ ONLY in how they build the
// *core.ProxyManager they pass to Router. If a change ever forces a
// provider-specific branch in here, the swap goal has regressed.
package api

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/gin-gonic/gin"
)

var (
	// SearchLimit: 5 requests per minute, burst 5.
	//
	// RefillGreedy drips continuously — one token back every 12s — so a caller
	// that paces itself is never rejected. RefillIntervally would instead return
	// all 5 at once on each minute boundary: same average rate, burstier clients.
	//
	// The rate is deliberately slow enough to see by hand. At 5/second a token
	// returns every 200ms, faster than you can type curl, and a working limiter
	// looks broken.
	SearchLimit = core.NewConfig(core.NewBandwidth(5).RefillGreedy(5, time.Minute))

	// HeavyLimit is a separate budget for expensive work: 10 tokens/minute, and
	// /api/heavy spends 5 per call — so two calls a minute, not ten.
	HeavyLimit = core.NewConfig(core.NewBandwidth(10).RefillGreedy(10, time.Minute))
)

// Router builds the API. backend is reported by /healthz so you can tell which
// provider a running process is actually using.
func Router(pm *core.ProxyManager, backend string) *gin.Engine {
	r := gin.Default()

	// Gin trusts ALL proxies by default, which means ClientIP() will believe an
	// X-Forwarded-For header sent by anyone. For a limiter keyed on IP that is a
	// bypass: forge the header, get a fresh bucket. nil = trust nobody, use the
	// real peer address. Behind a load balancer, list its CIDR here instead.
	if err := r.SetTrustedProxies(nil); err != nil {
		log.Fatalf("trusted proxies: %v", err)
	}

	// Unlimited: health checks must not consume tokens, or a liveness probe will
	// rate-limit your own orchestrator out of the service.
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "provider": backend})
	})

	api := r.Group("/api")
	{
		api.GET("/search", RateLimit(pm, "search", SearchLimit, 1), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"results": []string{"alpha", "beta"}})
		})

		// Note the distinct scope: a different Config must never share a key with
		// another. Cost-weighting is what makes a token bucket worth more than a
		// request counter.
		api.GET("/heavy", RateLimit(pm, "heavy", HeavyLimit, 5), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"report": "generated"})
		})
	}

	return r
}

// Serve runs h on $PORT (default 8080) until SIGINT/SIGTERM, then drains.
// It returns the context that is cancelled on signal, so callers can tie
// background work (e.g. the Postgres sweeper) to the same lifetime.
func Serve(ctx context.Context, h http.Handler) {
	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second, // don't let a slow client hold a conn open
	}

	// Bind before announcing. ListenAndServe in a goroutine would print
	// "listening" and only then discover the port is taken, which makes a failed
	// start look like a successful one — and a second replica silently not
	// running is exactly the bug you least want in a shared-limit setup.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	log.Printf("listening on %s", ln.Addr())

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down…")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
	log.Println("bye")
}

// SignalContext is cancelled on SIGINT/SIGTERM. Take it first, so a Ctrl-C
// during backend setup still exits cleanly.
func SignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}
