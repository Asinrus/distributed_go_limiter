package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/gin-gonic/gin"
)

// RateLimit returns Gin middleware that draws cost tokens from the caller's
// bucket and rejects with 429 when the bucket is empty.
//
// The bucket is looked up per request. That is cheap and deliberate: the handle
// is stateless and all the real state lives in the provider, so every replica of
// this service draws from the SAME bucket for a given key. That is the whole
// difference from the in-process limiter in Gin's security guide, which keeps a
// map[string]*rate.Limiter per process — unshared between replicas, and it grows
// for every client that ever calls you.
//
// Nothing in here is provider-specific: it behaves identically on in-memory,
// Redis and Postgres.
// scope namespaces the bucket key. It is not decoration: v1 does not store the
// Config alongside the state, so two limits with different Configs MUST NOT share
// a key — whoever writes last wins and both limits become wrong. One scope per
// distinct Config.
func RateLimit(pm *core.ProxyManager, scope string, cfg core.Config, cost int64) gin.HandlerFunc {
	limit := strconv.FormatInt(cfg.Bandwidth().Capacity(), 10)

	return func(c *gin.Context) {
		b, err := pm.Bucket(scope+":"+bucketKey(c), cfg)
		if err != nil {
			// Only an invalid Config gets here, and Config is fixed at startup —
			// so this fires on the first request after a bad deploy, not randomly.
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				gin.H{"error": "rate limiter misconfigured"})
			return
		}

		probe, err := b.TryConsumeAndReturnRemaining(c.Request.Context(), cost)
		if err != nil {
			// The store is unreachable. This is the decision every distributed
			// limiter forces and most get wrong by accident: failing CLOSED (here)
			// protects whatever you are limiting; failing OPEN (c.Next() instead)
			// protects your availability. Pick on purpose — there is no safe
			// default, and the right answer differs per endpoint.
			c.AbortWithStatusJSON(http.StatusServiceUnavailable,
				gin.H{"error": "rate limiter unavailable"})
			return
		}

		c.Header("X-RateLimit-Limit", limit)
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(probe.RemainingTokens, 10))

		if !probe.Consumed {
			// NanosToWaitForRefill is the exact wait for THIS request's cost, so a
			// 5-token call is told to wait longer than a 1-token one. Retry-After
			// is whole seconds, and 0 would invite an instant retry — floor at 1.
			retry := max(time.Duration(probe.NanosToWaitForRefill), time.Second)
			c.Header("Retry-After", strconv.Itoa(int(retry.Seconds())))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":               "rate limit exceeded",
				"retry_after_seconds": int(retry.Seconds()),
			})
			return
		}

		c.Next()
	}
}

// bucketKey picks which bucket a request draws from: the API key when one is
// presented, otherwise the client IP.
func bucketKey(c *gin.Context) string {
	if k := c.GetHeader("X-API-Key"); k != "" {
		return "key:" + k
	}
	// ClientIP() only honours X-Forwarded-For for proxies you have declared
	// trusted. See SetTrustedProxies in main.go — for an IP-keyed rate limiter
	// that setting is a security control, not a formality: trust every proxy and
	// a client can forge the header to mint itself a fresh bucket per request.
	return "ip:" + c.ClientIP()
}
