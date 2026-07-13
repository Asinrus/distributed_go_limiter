package core_test

import (
	"testing"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/asinrus/distributed_go_limiter/core/tck"
)

// AC-46: the in-memory provider passes the full conformance suite — the same suite
// the Redis and Postgres backends run. Only the constructor differs.
func TestInMemoryConformance(t *testing.T) {
	tck.Run(t, func() *core.ProxyManager {
		return core.NewInMemoryProxyManager()
	})
}
