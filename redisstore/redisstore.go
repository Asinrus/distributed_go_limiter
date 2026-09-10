// Package redisstore is the Redis storage backend for the distributed rate limiter.
// It implements core.CASStore using compare-and-swap: the token math runs in core
// (client-side), Redis only provides atomic get / conditional-set. See
// docs/client/redis.md.
package redisstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	"github.com/redis/go-redis/v9"
)

// CAS scripts: compare the whole stored blob to the expected old value, swap if
// equal. Redis runs each script atomically. Create-if-absent uses SET NX directly.
var (
	casScript = redis.NewScript(`
if redis.call('get', KEYS[1]) == ARGV[1] then
    redis.call('set', KEYS[1], ARGV[2])
    return 1
else
    return 0
end`)

	casScriptPX = redis.NewScript(`
if redis.call('get', KEYS[1]) == ARGV[1] then
    redis.call('psetex', KEYS[1], ARGV[3], ARGV[2])
    return 1
else
    return 0
end`)
)

type store struct {
	rdb    redis.Cmdable
	prefix string
}

var _ core.CASStore = (*store)(nil)

func (s *store) key(k string) string { return s.prefix + k }

func (s *store) GetState(ctx context.Context, key string) ([]byte, bool, error) {
	val, err := s.rdb.Get(ctx, s.key(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("redisstore: get %q: %w", key, err)
	}
	return val, true, nil
}

func (s *store) CompareAndSwap(ctx context.Context, key string, old, newState []byte, ttl time.Duration) (bool, error) {
	k := s.key(key)
	if old == nil {
		// Create only if absent (Redis rejects nil values, so this can't be a CAS).
		ok, err := s.rdb.SetNX(ctx, k, newState, ttl).Result()
		if err != nil {
			return false, fmt.Errorf("redisstore: setnx %q: %w", key, err)
		}
		return ok, nil
	}
	// Sub-millisecond TTLs would truncate to 0, which Redis rejects ("invalid
	// expire time"); round up to the smallest representable PX value instead.
	pttl := ttl.Milliseconds()
	if ttl > 0 && pttl == 0 {
		pttl = 1
	}
	var res int64
	var err error
	if pttl > 0 {
		res, err = casScriptPX.Run(ctx, s.rdb, []string{k}, old, newState, pttl).Int64()
	} else {
		res, err = casScript.Run(ctx, s.rdb, []string{k}, old, newState).Int64()
	}
	if err != nil {
		return false, fmt.Errorf("redisstore: cas %q: %w", key, err)
	}
	return res == 1, nil
}

func (s *store) Remove(ctx context.Context, key string) error {
	if err := s.rdb.Del(ctx, s.key(key)).Err(); err != nil {
		return fmt.Errorf("redisstore: remove %q: %w", key, err)
	}
	return nil
}

// Option configures the Redis ProxyManager.
type Option func(*config)

type config struct {
	prefix   string
	coreOpts []core.ProxyOption
}

// WithKeyPrefix namespaces bucket keys in Redis (e.g. "dgl:").
func WithKeyPrefix(p string) Option { return func(c *config) { c.prefix = p } }

// WithTTL sets an expire-after-write TTL on bucket state (0 = no expiry). It is
// sugar for WithProxyOption(core.WithExpireAfterWrite(d)) — the TTL must reach
// the store as well as core.ProxyManager's bookkeeping, so this is where both
// happen. If both WithTTL and an explicit core.WithExpireAfterWrite (passed via
// WithProxyOption) are given to NewProxyManager, whichever is passed last wins,
// same as any functional option — order in the call, not which helper it came
// through, decides.
func WithTTL(d time.Duration) Option {
	return func(c *config) { c.coreOpts = append(c.coreOpts, core.WithExpireAfterWrite(d)) }
}

// WithProxyOption forwards a core.ProxyOption — core.WithClock,
// core.WithMaxCASRetries, core.WithExpireAfterWrite, or any future one — to the
// underlying core.ProxyManager, so nothing core exposes is unreachable from this
// constructor. See WithTTL's doc for the case where both set a TTL.
func WithProxyOption(o core.ProxyOption) Option {
	return func(c *config) { c.coreOpts = append(c.coreOpts, o) }
}

// NewProxyManager builds a distributed ProxyManager backed by Redis. rdb may be a
// *redis.Client or *redis.ClusterClient. Calling code uses the returned
// core.ProxyManager exactly like any other backend.
func NewProxyManager(rdb redis.Cmdable, opts ...Option) *core.ProxyManager {
	c := config{}
	for _, o := range opts {
		o(&c)
	}
	s := &store{rdb: rdb, prefix: c.prefix}
	return core.NewCASProxyManager(s, c.coreOpts...)
}
