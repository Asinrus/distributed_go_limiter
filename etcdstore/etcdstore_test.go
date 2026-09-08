package etcdstore_test

import (
	"context"
	"flag"
	"fmt"
	"os"
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

// etcdCli is shared by every test in this file: one container for the whole
// package rather than one per test. Tests stay isolated by using distinct
// keys (core/tck generates its own; the rest use fixed, non-overlapping
// names), so a shared keyspace never leaks state between tests.
var etcdCli *clientv3.Client

// TestMain starts a single etcd container for the package (skipped in
// -short mode, same as every other backend's container tests) and tears it
// down after all tests finish.
func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(m.Run())
	}

	ctx := context.Background()

	// testcontainers-go v0.33.0 has no ready-made etcd module, so the
	// container is built directly. Image is fully qualified because the
	// local registries.conf only searches docker.io.
	req := testcontainers.ContainerRequest{
		Image:        "quay.io/coreos/etcd:v3.5.16",
		ExposedPorts: []string{"2379/tcp"},
		Cmd: []string{
			"etcd",
			"--listen-client-urls", "http://0.0.0.0:2379",
			"--advertise-client-urls", "http://0.0.0.0:2379",
			// Lower than etcd's defaults (1000ms/100ms) to shrink the
			// server-enforced minimum lease TTL, so TestEtcdExpiration
			// doesn't have to sleep past the default ~3s floor. See the
			// comment on that sleep for what this buys.
			"--election-timeout", "250",
			"--heartbeat-interval", "50",
		},
		// SkipInternalCheck: podman's rootless socket rejects the exec-based
		// internal probe testcontainers otherwise runs first.
		WaitingFor: wait.ForListeningPort("2379/tcp").SkipInternalCheck(),
	}
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "etcdstore: start container:", err)
		os.Exit(1)
	}

	endpoint, err := ctr.PortEndpoint(ctx, "2379/tcp", "http")
	if err != nil {
		fmt.Fprintln(os.Stderr, "etcdstore: container endpoint:", err)
		os.Exit(1)
	}

	etcdCli, err = clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "etcdstore: connect client:", err)
		os.Exit(1)
	}

	code := m.Run()

	_ = etcdCli.Close()
	_ = ctr.Terminate(ctx)
	os.Exit(code)
}

func startEtcd(t *testing.T) *clientv3.Client {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping container test in -short mode")
	}
	return etcdCli
}

// AC-41..44 on real etcd — the same suite the in-memory, Redis, and Postgres
// backends run.
func TestEtcdConformance(t *testing.T) {
	cli := startEtcd(t)
	tck.Run(t, func() *core.ProxyManager {
		return etcdstore.NewProxyManager(cli)
	})
}

// countLeases returns the number of leases currently live on the cluster.
// Every write in these tests goes through a TTL'd bucket, so a live lease
// count is a direct proxy for "how many server-side lease objects has this
// code left behind" — the thing etcdstore-lease-leak.md is about.
func countLeases(t *testing.T, ctx context.Context, cli *clientv3.Client) int {
	t.Helper()
	resp, err := cli.Leases(ctx)
	require.NoError(t, err)
	return len(resp.Leases)
}

// TestEtcdLeaseCountBoundedByLiveKeys is the assertion that fails against the
// pre-fix code: it grants a fresh lease on every write, so N sequential
// writes to one key left N live leases (the old lease displaced by each
// overwrite was never revoked, only left to expire on its own). A single key
// must produce a bounded lease count regardless of how many times it is
// written.
func TestEtcdLeaseCountBoundedByLiveKeys(t *testing.T) {
	cli := startEtcd(t)
	ctx := context.Background()
	pm := etcdstore.NewProxyManager(cli, etcdstore.WithTTL(time.Hour))
	cfg := core.NewConfig(core.NewBandwidth(1000).RefillGreedy(1, time.Hour))

	b, err := pm.Bucket("lease-bound", cfg)
	require.NoError(t, err)

	before := countLeases(t, ctx, cli)

	const writes = 20
	for i := 0; i < writes; i++ {
		ok, err := b.TryConsume(ctx, 1)
		require.NoError(t, err)
		require.True(t, ok)
	}

	assert.LessOrEqual(t, countLeases(t, ctx, cli)-before, 1,
		"lease count must grow by at most the one live key, not by write count (%d)", writes)
}

// TestEtcdLostCASRaceLeavesNoLease drives concurrent writers at the same key
// so some of core's internal CAS attempts lose their race. A lease granted
// for a losing attempt is never attached to a key by the Txn (Then never
// runs), so if it isn't revoked explicitly it survives as an orphan until its
// TTL lapses on its own. The lease count after all writers finish must still
// be bounded by the one live key, not by the number of attempts (winners +
// losers) it took to get there.
func TestEtcdLostCASRaceLeavesNoLease(t *testing.T) {
	cli := startEtcd(t)
	ctx := context.Background()
	pm := etcdstore.NewProxyManager(cli, etcdstore.WithTTL(time.Hour))
	cfg := core.NewConfig(core.NewBandwidth(1000).RefillGreedy(1, time.Hour))

	b, err := pm.Bucket("lease-race", cfg)
	require.NoError(t, err)

	before := countLeases(t, ctx, cli)

	const writers = 10
	done := make(chan struct{}, writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			ok, err := b.TryConsume(ctx, 1)
			assert.NoError(t, err)
			assert.True(t, ok)
		}()
	}
	for i := 0; i < writers; i++ {
		<-done
	}

	assert.LessOrEqual(t, countLeases(t, ctx, cli)-before, 1,
		"a lost CAS race must not leave an orphan lease behind")
}

// TestEtcdLeaseCacheAcrossWriters drives two independent *ProxyManager
// instances (standing in for two processes, since each owns its own private
// leases cache) alternately writing the same key. A cache keyed by "the
// lease this instance last wrote" cannot see the other instance's lease: it
// would keep calling KeepAliveOnce on its own now-detached lease forever
// (an orphan that never expires) while never refreshing the lease actually
// attached to the key. Bounding the live lease count here is the proof the
// cache is instead sourced from what GetState observes attached to the key,
// not from local write history.
func TestEtcdLeaseCacheAcrossWriters(t *testing.T) {
	cli := startEtcd(t)
	ctx := context.Background()
	cfg := core.NewConfig(core.NewBandwidth(1000).RefillGreedy(1, time.Hour))

	pmA := etcdstore.NewProxyManager(cli, etcdstore.WithTTL(time.Hour))
	pmB := etcdstore.NewProxyManager(cli, etcdstore.WithTTL(time.Hour))
	bA, err := pmA.Bucket("lease-cross-writer", cfg)
	require.NoError(t, err)
	bB, err := pmB.Bucket("lease-cross-writer", cfg)
	require.NoError(t, err)

	before := countLeases(t, ctx, cli)

	const rounds = 10
	for i := 0; i < rounds; i++ {
		ok, err := bA.TryConsume(ctx, 1)
		require.NoError(t, err)
		require.True(t, ok)

		ok, err = bB.TryConsume(ctx, 1)
		require.NoError(t, err)
		require.True(t, ok)
	}

	assert.LessOrEqual(t, countLeases(t, ctx, cli)-before, 1,
		"alternating writers on the same key must converge on one lease, not one per writer")
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

	// etcd enforces a server-side minimum lease TTL (~3x its election
	// timeout, regardless of the seconds requested via Grant), so wait past
	// that rather than the requested 1s. startEtcd's container runs with a
	// lowered election-timeout/heartbeat-interval to shrink that floor;
	// measured lease expiry against it lands around 1.3-1.5s, so 3s leaves a
	// comfortable margin without reintroducing the ~3-4s a default-timeout
	// cluster would need.
	time.Sleep(3 * time.Second)

	av, err = b.AvailableTokens(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(10), av, "state expired -> fresh full bucket")
}
