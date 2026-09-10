package mongostore_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/asinrus/distributed_go_limiter/core/storetest"
	"github.com/asinrus/distributed_go_limiter/mongostore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcmongodb "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// mongoColl is shared by every test in this file: one container for the
// whole package rather than one per test. Tests stay isolated by using
// distinct document keys (core/storetest generates its own; the rest use fixed,
// non-overlapping names), so a shared collection never leaks state between
// tests.
var mongoColl *mongo.Collection

// TestMain starts a single MongoDB container for the package (skipped in
// -short mode, same as every other backend's container tests) and tears it
// down after all tests finish.
func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(m.Run())
	}

	ctx := context.Background()

	ctr, err := tcmongodb.Run(ctx, "mongo:7")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mongostore: start container:", err)
		os.Exit(1)
	}

	uri, err := ctr.ConnectionString(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mongostore: connection string:", err)
		os.Exit(1)
	}
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		fmt.Fprintln(os.Stderr, "mongostore: connect client:", err)
		os.Exit(1)
	}
	if err := client.Ping(ctx, nil); err != nil {
		fmt.Fprintln(os.Stderr, "mongostore: ping:", err)
		os.Exit(1)
	}

	mongoColl = client.Database("test").Collection("bucket")
	if err := mongostore.EnsureIndexes(ctx, mongoColl); err != nil {
		fmt.Fprintln(os.Stderr, "mongostore: ensure indexes:", err)
		os.Exit(1)
	}

	code := m.Run()

	_ = client.Disconnect(ctx)
	_ = ctr.Terminate(ctx)
	os.Exit(code)
}

func startMongo(t *testing.T) *mongo.Collection {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping container test in -short mode")
	}
	return mongoColl
}

// AC-41..44 on real MongoDB — the same suite the in-memory, Redis and Postgres
// backends run.
func TestMongoConformance(t *testing.T) {
	coll := startMongo(t)
	storetest.Run(t, func() *core.ProxyManager {
		return mongostore.NewProxyManager(coll)
	})
}

// AC-45: expired state reads as a fresh bucket, matching the Redis/Postgres
// expiry semantics even though Mongo's TTL reaper has not run yet.
func TestMongoExpiration(t *testing.T) {
	coll := startMongo(t)
	ctx := context.Background()
	pm := mongostore.NewProxyManager(coll, mongostore.WithTTL(1*time.Second))
	cfg := core.NewConfig(core.NewBandwidth(10).RefillGreedy(1, time.Hour))

	b, err := pm.Bucket("exp", cfg)
	require.NoError(t, err)

	ok, err := b.TryConsume(ctx, 4)
	require.NoError(t, err)
	require.True(t, ok)

	av, err := b.AvailableTokens(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(6), av)

	time.Sleep(1500 * time.Millisecond)

	av, err = b.AvailableTokens(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(10), av, "state expired -> fresh full bucket")
}

// docs/adr/0012: create-if-absent must match a not-yet-reaped expired document,
// not just a missing one, or the CAS executor retries into a permanent failure.
func TestMongoCreateOverwritesUnreapedExpiredDoc(t *testing.T) {
	coll := startMongo(t)
	ctx := context.Background()
	pm := mongostore.NewProxyManager(coll, mongostore.WithTTL(1*time.Second))
	cfg := core.NewConfig(core.NewBandwidth(10).RefillGreedy(1, time.Hour))

	b, err := pm.Bucket("stale", cfg)
	require.NoError(t, err)
	ok, err := b.TryConsume(ctx, 4)
	require.NoError(t, err)
	require.True(t, ok)

	time.Sleep(1500 * time.Millisecond)

	// Document is still physically present (reaper runs ~once a minute) but
	// logically expired; a fresh TryConsume must still succeed.
	count, err := coll.CountDocuments(ctx, bson.M{"_id": "stale"})
	require.NoError(t, err)
	require.Equal(t, int64(1), count, "expired doc not yet reaped")

	ok, err = b.TryConsume(ctx, 3)
	require.NoError(t, err)
	require.True(t, ok, "create-if-absent must treat an expired doc as absent")

	av, err := b.AvailableTokens(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(7), av)
}
