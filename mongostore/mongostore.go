// Package mongostore is the MongoDB storage backend for the distributed rate
// limiter. It implements core.CASStore using a filter-based ReplaceOne: the
// token math runs in core (client-side), MongoDB only provides an atomic
// conditional replace keyed by _id. See docs/client/mongodb.md and
// docs/adr/0012-ttl-is-not-a-correctness-mechanism.md for why expiry is
// enforced in the query filter rather than left to the TTL index.
package mongostore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// doc is the one-document-per-bucket shape: { _id, state, expiresAt }.
// expiresAt is absent (not null) when the document has no expiry.
type doc struct {
	ID        string    `bson:"_id"`
	State     []byte    `bson:"state"`
	ExpiresAt time.Time `bson:"expiresAt,omitempty"`
}

// live matches a document that is not expired: expiresAt absent, or in the
// future. Reused by reads and by the create-if-absent write filter.
func live(now time.Time) bson.M {
	return bson.M{"$or": bson.A{
		bson.M{"expiresAt": bson.M{"$exists": false}},
		bson.M{"expiresAt": bson.M{"$gt": now}},
	}}
}

// Store is the MongoDB core.CASStore. Obtain one with NewStore; most callers
// want NewProxyManager instead.
type Store struct {
	coll *mongo.Collection
	now  func() time.Time

	// coreOpts stages core.ProxyOption values (including the TTL translated by
	// WithTTL) until NewProxyManager forwards them to core.NewCASProxyManager.
	// It has no effect on a Store built via NewStore directly and used as a
	// bare core.CASStore — expiry there comes from the ttl core/command.go
	// passes into CompareAndSwap, not from this field.
	coreOpts []core.ProxyOption
}

var _ core.CASStore = (*Store)(nil)

func (s *Store) GetState(ctx context.Context, key string) ([]byte, bool, error) {
	filter := bson.M{"_id": key}
	for k, v := range live(s.now()) {
		filter[k] = v
	}
	var d doc
	err := s.coll.FindOne(ctx, filter).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("mongostore: get %q: %w", key, err)
	}
	return d.State, true, nil
}

func (s *Store) CompareAndSwap(ctx context.Context, key string, old, newState []byte, ttl time.Duration) (bool, error) {
	now := s.now()
	newDoc := doc{ID: key, State: newState}
	if ttl > 0 {
		newDoc.ExpiresAt = now.Add(ttl)
	}

	var filter bson.M
	upsert := false
	if old == nil {
		// Create only if absent or expired: a plain "absent" filter would fail
		// forever against a document the TTL reaper has not yet collected.
		filter = bson.M{"_id": key, "$nor": bson.A{live(now)}}
		upsert = true
	} else {
		filter = bson.M{"_id": key, "state": old}
		for k, v := range live(now) {
			filter[k] = v
		}
	}

	res, err := s.coll.ReplaceOne(ctx, filter, newDoc, options.Replace().SetUpsert(upsert))
	if err != nil {
		if upsert && mongo.IsDuplicateKeyError(err) {
			// Another writer won the create race between our filter match and
			// the upsert — not an error, just a lost CAS.
			return false, nil
		}
		return false, fmt.Errorf("mongostore: cas %q: %w", key, err)
	}
	return res.MatchedCount+res.UpsertedCount == 1, nil
}

func (s *Store) Remove(ctx context.Context, key string) error {
	if _, err := s.coll.DeleteOne(ctx, bson.M{"_id": key}); err != nil {
		return fmt.Errorf("mongostore: remove %q: %w", key, err)
	}
	return nil
}

// Option configures the MongoDB backend.
type Option func(*Store)

// WithTTL sets an expire-after-write TTL on bucket state (0 = no expiry). It is
// sugar for WithProxyOption(core.WithExpireAfterWrite(d)).
func WithTTL(d time.Duration) Option {
	return func(s *Store) { s.coreOpts = append(s.coreOpts, core.WithExpireAfterWrite(d)) }
}

// WithProxyOption forwards a core.ProxyOption — core.WithClock,
// core.WithMaxCASRetries, core.WithExpireAfterWrite, or any future one — to the
// underlying core.ProxyManager, so nothing core exposes is unreachable from this
// constructor.
func WithProxyOption(o core.ProxyOption) Option {
	return func(s *Store) { s.coreOpts = append(s.coreOpts, o) }
}

// IndexModels returns the TTL index used by EnsureIndexes: storage reclamation
// only, since expiry correctness comes from the query filter in GetState and
// CompareAndSwap, not from the reaper.
func IndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{{
		Keys: bson.D{{Key: "expiresAt", Value: 1}},
		Options: options.Index().
			SetExpireAfterSeconds(0).
			SetPartialFilterExpression(bson.M{"expiresAt": bson.M{"$exists": true}}),
	}}
}

// EnsureIndexes creates the TTL index on coll. Idempotent, safe to call on
// every start.
func EnsureIndexes(ctx context.Context, coll *mongo.Collection) error {
	if _, err := coll.Indexes().CreateMany(ctx, IndexModels()); err != nil {
		return fmt.Errorf("mongostore: ensure indexes: %w", err)
	}
	return nil
}

// NewStore returns the raw CASStore. Most callers want NewProxyManager instead.
func NewStore(coll *mongo.Collection, opts ...Option) *Store {
	s := &Store{coll: coll, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	return s
}

// NewProxyManager builds a distributed ProxyManager backed by MongoDB. Call
// EnsureIndexes first (or create the index yourself). Calling code uses the
// returned core.ProxyManager exactly like any other backend.
func NewProxyManager(coll *mongo.Collection, opts ...Option) *core.ProxyManager {
	s := NewStore(coll, opts...)
	return core.NewCASProxyManager(s, s.coreOpts...)
}
