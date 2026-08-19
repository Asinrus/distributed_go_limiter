package mongostore_test

import (
	"context"
	"testing"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/asinrus/distributed_go_limiter/core/tck"
	"github.com/asinrus/distributed_go_limiter/mongostore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcmongodb "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func startMongo(t *testing.T) *mongo.Collection {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping container test in -short mode")
	}
	ctx := context.Background()
	ctr, err := tcmongodb.Run(ctx, "mongo:7")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	uri, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(ctx) })
	require.NoError(t, client.Ping(ctx, nil))

	coll := client.Database("test").Collection("bucket")
	require.NoError(t, mongostore.EnsureIndexes(ctx, coll))
	return coll
}

// AC-41..44 on real MongoDB — the same suite the in-memory, Redis and Postgres
// backends run.
func TestMongoConformance(t *testing.T) {
	coll := startMongo(t)
	tck.Run(t, func() *core.ProxyManager {
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
