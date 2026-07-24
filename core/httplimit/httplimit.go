// Package httplimit adapts a core.Bucket into HTTP middleware shaped
// func(http.Handler) http.Handler — the shape net/http and any router built on
// it (chi, gorilla/mux, ...) consume natively. Frameworks with their own
// handler type (Gin, Echo, Fiber) need a short adapter; see the package
// README for one per framework. Stdlib only: importing this package adds no
// dependency beyond core itself.
//
// Configuration is fixed when Middleware is built and never changes
// afterward — there is no reload, no mutation, nothing to synchronize at
// runtime. Build it once at startup and reuse the returned wrapper for the
// life of the process, the same way core.Config and every backend's
// functional options are captured once at construction.
package httplimit

import (
	"net/http"
	"strconv"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
)

// BucketFunc resolves the core.Bucket a request should draw tokens from. It
// decides identity — which bucket — not policy: capacity and refill rate live
// in the Config already baked into the Bucket it returns.
type BucketFunc func(r *http.Request) (core.Bucket, error)

// Fixed always returns the same Bucket: one shared limit for every request,
// e.g. a single LocalBucket guarding an endpoint, or one proxyBucket shared
// across an app that doesn't need per-client limits.
func Fixed(b core.Bucket) BucketFunc {
	return func(*http.Request) (core.Bucket, error) { return b, nil }
}

// ByKey resolves a per-client Bucket from a ProxyManager: keyFn extracts the
// caller's identity (client IP, API key, tenant ID, ...) and cfg is the policy
// every one of those buckets shares. cfg is captured once, here, not read per
// request — nodes sharing a key must use the same Config, since v1 does not
// store it alongside the state.
func ByKey(pm *core.ProxyManager, cfg core.Config, keyFn func(*http.Request) string) BucketFunc {
	return func(r *http.Request) (core.Bucket, error) {
		return pm.Bucket(keyFn(r), cfg)
	}
}

type options struct {
	cost      int64
	onLimited func(w http.ResponseWriter, r *http.Request, probe core.ConsumptionProbe)
	onError   func(w http.ResponseWriter, r *http.Request, err error)
}

// Option configures Middleware. Every Option is applied once, at construction.
type Option func(*options)

// Cost sets how many tokens each request consumes (default 1). Use this to
// price expensive endpoints out of a shared budget faster than cheap ones.
func Cost(n int64) Option { return func(o *options) { o.cost = n } }

// OnLimited overrides the response written when a request is rejected.
// Default: 429 Too Many Requests with a Retry-After header sized to the exact
// wait this request's cost requires.
func OnLimited(fn func(http.ResponseWriter, *http.Request, core.ConsumptionProbe)) Option {
	return func(o *options) { o.onLimited = fn }
}

// OnError overrides the response written when resolving or checking the
// bucket fails — most often the store is unreachable. Default: fail closed,
// 503 Service Unavailable. Failing open (call next instead) is equally
// defensible and depends on what the endpoint protects — that is exactly why
// this is a hook and not a default you cannot change.
func OnError(fn func(http.ResponseWriter, *http.Request, error)) Option {
	return func(o *options) { o.onError = fn }
}

func defaultOnLimited(w http.ResponseWriter, _ *http.Request, probe core.ConsumptionProbe) {
	retry := max(time.Duration(probe.NanosToWaitForRefill), time.Second)
	w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())))
	http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
}

func defaultOnError(w http.ResponseWriter, _ *http.Request, err error) {
	http.Error(w, err.Error(), http.StatusServiceUnavailable)
}

// Middleware returns HTTP middleware backed by bucketFor: on each request it
// resolves a Bucket, spends Cost tokens, and either calls next or invokes
// OnLimited / OnError. It carries X-RateLimit-Remaining on every allowed
// response.
func Middleware(bucketFor BucketFunc, opts ...Option) func(http.Handler) http.Handler {
	o := options{cost: 1, onLimited: defaultOnLimited, onError: defaultOnError}
	for _, opt := range opts {
		opt(&o)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, err := bucketFor(r)
			if err != nil {
				o.onError(w, r, err)
				return
			}

			probe, err := b.TryConsumeAndReturnRemaining(r.Context(), o.cost)
			if err != nil {
				o.onError(w, r, err)
				return
			}

			w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(probe.RemainingTokens, 10))
			if !probe.Consumed {
				o.onLimited(w, r, probe)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
