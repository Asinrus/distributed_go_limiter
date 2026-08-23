// Package etcdstore is the etcd storage backend for the distributed rate
// limiter. It implements core.CASStore using etcd's Txn compare against the
// stored value bytes: the token math runs in core (client-side), etcd only
// provides atomic get / conditional-put. See docs/client/etcd.md.
package etcdstore

import (
	"context"
	"fmt"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type store struct {
	cli    *clientv3.Client
	prefix string
}

var _ core.CASStore = (*store)(nil)

func (s *store) key(k string) string { return s.prefix + k }

func (s *store) GetState(ctx context.Context, key string) ([]byte, bool, error) {
	resp, err := s.cli.Get(ctx, s.key(key))
	if err != nil {
		return nil, false, fmt.Errorf("etcdstore: get %q: %w", key, err)
	}
	if len(resp.Kvs) == 0 {
		return nil, false, nil
	}
	return resp.Kvs[0].Value, true, nil
}

func (s *store) CompareAndSwap(ctx context.Context, key string, old, newState []byte, ttl time.Duration) (bool, error) {
	k := s.key(key)

	putOpts, err := s.putOpts(ctx, ttl)
	if err != nil {
		return false, fmt.Errorf("etcdstore: grant lease %q: %w", key, err)
	}

	var cmp clientv3.Cmp
	if old == nil {
		cmp = clientv3.Compare(clientv3.CreateRevision(k), "=", 0)
	} else {
		cmp = clientv3.Compare(clientv3.Value(k), "=", string(old))
	}

	resp, err := s.cli.Txn(ctx).
		If(cmp).
		Then(clientv3.OpPut(k, string(newState), putOpts...)).
		Commit()
	if err != nil {
		return false, fmt.Errorf("etcdstore: cas %q: %w", key, err)
	}
	return resp.Succeeded, nil
}

// putOpts grants a fresh lease for ttl > 0. A lease is etcd's server-side TTL
// handle: every key attached to it is deleted when the lease expires. etcd's
// lease granularity is whole seconds with a minimum of 1, so sub-second TTLs
// round up. The lease is never kept alive: expire-after-write means each
// write attaches its own lease, and the lease displaced by an overwrite is
// left to expire on its own rather than revoked explicitly.
func (s *store) putOpts(ctx context.Context, ttl time.Duration) ([]clientv3.OpOption, error) {
	if ttl <= 0 {
		return nil, nil
	}
	seconds := int64(ttl / time.Second)
	if ttl%time.Second != 0 || seconds == 0 {
		seconds++
	}
	lease, err := s.cli.Grant(ctx, seconds)
	if err != nil {
		return nil, err
	}
	return []clientv3.OpOption{clientv3.WithLease(lease.ID)}, nil
}

func (s *store) Remove(ctx context.Context, key string) error {
	if _, err := s.cli.Delete(ctx, s.key(key)); err != nil {
		return fmt.Errorf("etcdstore: remove %q: %w", key, err)
	}
	return nil
}

// Option configures the etcd ProxyManager.
type Option func(*config)

type config struct {
	prefix string
	ttl    time.Duration
}

// WithKeyPrefix namespaces bucket keys in etcd (e.g. "dgl:").
func WithKeyPrefix(p string) Option { return func(c *config) { c.prefix = p } }

// WithTTL sets an expire-after-write TTL on bucket state (0 = no expiry). etcd
// leases are granted in whole seconds with a minimum of 1s, so a TTL under 1s
// (e.g. 200ms) is rounded up to 1s.
func WithTTL(d time.Duration) Option { return func(c *config) { c.ttl = d } }

// NewProxyManager builds a distributed ProxyManager backed by etcd. cli's
// lifetime is owned by the caller, as with redis.Cmdable and *pgxpool.Pool.
func NewProxyManager(cli *clientv3.Client, opts ...Option) *core.ProxyManager {
	c := config{}
	for _, o := range opts {
		o(&c)
	}
	s := &store{cli: cli, prefix: c.prefix}
	return core.NewCASProxyManager(s, core.WithExpireAfterWrite(c.ttl))
}
