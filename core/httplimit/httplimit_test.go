package httplimit_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/asinrus/distributed_go_limiter/core/httplimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slowConfig has a capacity but a negligible refill rate, so token accounting
// is exact over a test's short wall-clock duration — no fake clock needed.
func slowConfig(capacity int64) core.Config {
	return core.NewConfig(core.NewBandwidth(capacity).RefillGreedy(1, time.Hour))
}

func okHandler() (http.Handler, *bool) {
	called := false
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}), &called
}

func TestMiddleware_Allows(t *testing.T) {
	b, err := core.NewLocalBucket(slowConfig(2))
	require.NoError(t, err)

	next, called := okHandler()
	h := httplimit.Middleware(httplimit.Fixed(b))(next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.True(t, *called, "next must run when tokens are available")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "1", rec.Header().Get("X-RateLimit-Remaining"))
}

func TestMiddleware_LimitsWithDefaults(t *testing.T) {
	b, err := core.NewLocalBucket(slowConfig(1))
	require.NoError(t, err)

	next, called := okHandler()
	h := httplimit.Middleware(httplimit.Fixed(b))(next)

	// First request spends the only token.
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	*called = false

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.False(t, *called, "next must not run once the bucket is empty")
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestMiddleware_OnLimitedOverride(t *testing.T) {
	b, err := core.NewLocalBucket(slowConfig(1))
	require.NoError(t, err)
	_, _ = b.TryConsume(context.Background(), 1) // exhaust the only token

	fired := false
	h := httplimit.Middleware(httplimit.Fixed(b),
		httplimit.OnLimited(func(w http.ResponseWriter, _ *http.Request, probe core.ConsumptionProbe) {
			fired = true
			assert.False(t, probe.Consumed)
			w.WriteHeader(http.StatusTeapot)
		}),
	)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next must not run")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.True(t, fired)
	assert.Equal(t, http.StatusTeapot, rec.Code)
}

func TestMiddleware_OnErrorOverride(t *testing.T) {
	boom := errors.New("store unreachable")
	broken := func(*http.Request) (core.Bucket, error) { return nil, boom }

	fired := false
	h := httplimit.Middleware(broken,
		httplimit.OnError(func(w http.ResponseWriter, _ *http.Request, err error) {
			fired = true
			assert.ErrorIs(t, err, boom)
			w.WriteHeader(http.StatusBadGateway)
		}),
	)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next must not run")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.True(t, fired)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestMiddleware_ErrorDefaultsToFailClosed(t *testing.T) {
	broken := func(*http.Request) (core.Bucket, error) { return nil, errors.New("boom") }
	h := httplimit.Middleware(broken)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next must not run")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestMiddleware_Cost(t *testing.T) {
	b, err := core.NewLocalBucket(slowConfig(5))
	require.NoError(t, err)

	next, _ := okHandler()
	h := httplimit.Middleware(httplimit.Fixed(b), httplimit.Cost(3))(next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, "2", rec.Header().Get("X-RateLimit-Remaining"), "5 - cost(3) = 2")

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "2 remaining < cost(3)")
}

func TestByKey_IsolatesClients(t *testing.T) {
	pm := core.NewInMemoryProxyManager()
	cfg := slowConfig(1)

	bucketFor := httplimit.ByKey(pm, cfg, func(r *http.Request) string {
		return r.Header.Get("X-Client")
	})
	h := httplimit.Middleware(bucketFor)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := func(client string) int {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Client", client)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}

	assert.Equal(t, http.StatusOK, req("alice"))
	assert.Equal(t, http.StatusTooManyRequests, req("alice"), "alice's single token is spent")
	assert.Equal(t, http.StatusOK, req("bob"), "bob has an independent bucket")
}

func TestFixed_SharesOneBucketAcrossClients(t *testing.T) {
	b, err := core.NewLocalBucket(slowConfig(1))
	require.NoError(t, err)

	h := httplimit.Middleware(httplimit.Fixed(b))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := func(client string) int {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Client", client)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}

	assert.Equal(t, http.StatusOK, req("alice"))
	assert.Equal(t, http.StatusTooManyRequests, req("bob"), "Fixed ignores identity: one shared budget")
}
