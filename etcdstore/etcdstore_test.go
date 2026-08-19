package etcdstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/asinrus/distributed_go_limiter/core/tck"
	"github.com/asinrus/distributed_go_limiter/etcdstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// testcontainers-go v0.33.0 has no ready-made etcd module, so the container is
// built directly. Image is fully qualified because the local registries.conf
// only searches docker.io.
func startEtcd(t *testing.T) *clientv3.Client {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping container test in -short mode")
	}
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "quay.io/coreos/etcd:v3.5.16",
		ExposedPorts: []string{"2379/tcp"},
		Cmd: []string{
			"etcd",
			"--listen-client-urls", "http://0.0.0.0:2379",
			"--advertise-client-urls", "http://0.0.0.0:2379",
		},
		// SkipInternalCheck: podman's rootless socket rejects the exec-based
		// internal probe testcontainers otherwise runs first.
		WaitingFor: wait.ForListeningPort("2379/tcp").SkipInternalCheck(),
	}
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	endpoint, err := ctr.PortEndpoint(ctx, "2379/tcp", "http")
	require.NoError(t, err)

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

// AC-41..44 on real etcd — the same suite the in-memory, Redis, and Postgres
// backends run.
func TestEtcdConformance(t *testing.T) {
	cli := startEtcd(t)
	tck.Run(t, func() *core.ProxyManager {
		return etcdstore.NewProxyManager(cli)
	})
}

// AC-45: expire-after-write on etcd (short TTL + real wait for lease expiry).
func TestEtcdExpiration(t *testing.T) {
	cli := startEtcd(t)
	ctx := context.Background()
	pm := etcdstore.NewProxyManager(cli, etcdstore.WithTTL(1*time.Second))
	cfg := core.NewConfig(core.NewBandwidth(10).RefillGreedy(1, time.Hour))

	b, err := pm.Bucket("exp", cfg)
	require.NoError(t, err)

	ok, err := b.TryConsume(ctx, 4)
	require.NoError(t, err)
	require.True(t, ok)

	av, err := b.AvailableTokens(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(6), av)

	// etcd enforces a server-side minimum lease TTL (~3x its election timeout,
	// regardless of the seconds requested via Grant), so wait past that rather
	// than the requested 1s.
	time.Sleep(4 * time.Second)

	av, err = b.AvailableTokens(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(10), av, "state expired -> fresh full bucket")
}
