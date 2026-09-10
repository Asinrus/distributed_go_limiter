package core_test

import (
	"testing"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/asinrus/distributed_go_limiter/core/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC-46: the in-memory provider passes the full conformance suite — the same suite
// the Redis and Postgres backends run. Only the constructor differs.
func TestInMemoryConformance(t *testing.T) {
	storetest.Run(t, func(now func() int64) *core.ProxyManager {
		return core.NewInMemoryProxyManager(core.WithClock(now))
	})
}

// An invalid config is rejected at construction on the distributed path too, not
// just LocalBucket — same error, same place, whichever provider you swap in.
func TestInvalidConfig_ProxyBucket(t *testing.T) {
	pm := core.NewInMemoryProxyManager()

	t.Run("zero refill period", func(t *testing.T) {
		_, err := pm.Bucket("bad-1", core.NewConfig(core.NewBandwidth(10)))
		assert.ErrorIs(t, err, core.ErrInvalidConfig)
	})

	t.Run("non-positive capacity", func(t *testing.T) {
		_, err := pm.Bucket("bad-2", core.NewConfig(core.NewBandwidth(0).RefillGreedy(1, time.Second)))
		assert.ErrorIs(t, err, core.ErrInvalidConfig)
	})

	t.Run("valid config builds", func(t *testing.T) {
		b, err := pm.Bucket("good", core.NewConfig(core.NewBandwidth(10).RefillGreedy(1, time.Second)))
		require.NoError(t, err)
		require.NotNil(t, b)
	})
}
