// Package etcdstore is the etcd storage backend for the distributed rate
// limiter. It implements core.CASStore using etcd's Txn compare against the
// stored value bytes: the token math runs in core (client-side), etcd only
// provides atomic get / conditional-put. See docs/client/etcd.md.
package etcdstore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/asinrus/distributed_go_limiter/core"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type store struct {
	cli    *clientv3.Client
	prefix string

	// leases caches, per key, the lease ID GetState most recently observed
	// attached to that key in etcd — not a lease this store granted itself.
	// The executor always calls GetState immediately before CompareAndSwap
	// (core/command.go), so by the time a write uses the cache it reflects
	// whichever writer — this store or another process sharing the key —
	// last attached a lease, and self-corrects every read/write cycle. That
	// is what makes it safe when several stores write the same key: a
	// per-writer "lease I last wrote" cache cannot see another writer's
	// lease and would keep refreshing its own orphan forever.
	//
	// Entries are never evicted for a key this store stops touching, so
	// the map grows with the set of distinct keys ever seen (bytes per
	// key, not a full etcd lease — separate from the etcd-side lease
	// count this cache bounds). Revisit with an eviction policy if a
	// workload churns through many short-lived keys on one long-lived
	// store.
	mu     sync.Mutex
	leases map[string]clientv3.LeaseID
}

var _ core.CASStore = (*store)(nil)

func (s *store) key(k string) string { return s.prefix + k }

func (s *store) GetState(ctx context.Context, key string) ([]byte, bool, error) {
	resp, err := s.cli.Get(ctx, s.key(key))
	if err != nil {
		return nil, false, fmt.Errorf("etcdstore: get %q: %w", key, err)
	}
	if len(resp.Kvs) == 0 {
		s.clearLease(key)
		return nil, false, nil
	}
	if lease := clientv3.LeaseID(resp.Kvs[0].Lease); lease != 0 {
		s.setLease(key, lease)
	} else {
		s.clearLease(key)
	}
	return resp.Kvs[0].Value, true, nil
}

func (s *store) CompareAndSwap(ctx context.Context, key string, old, newState []byte, ttl time.Duration) (bool, error) {
	k := s.key(key)

	putOpts, freshLease, err := s.putOpts(ctx, key, ttl)
	if err != nil {
		return false, fmt.Errorf("etcdstore: lease %q: %w", key, err)
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

	if !resp.Succeeded {
		// This attempt lost its race: Then never ran, so a freshly granted
		// lease was never attached to a key. Revoke it now rather than
		// leaving it to expire on its own — otherwise live lease count
		// grows with write *attempts*, not live keys. A failed revoke is
		// only a best effort: core/store.go's contract is that
		// swapped=false, err=nil means "lost the race, retry", and
		// core/command.go aborts without retrying on any non-nil error. A
		// transient Revoke failure must not turn a normal lost race into a
		// hard failure for state that is perfectly fine — the orphaned
		// lease still expires on its own TTL either way.
		if freshLease != 0 {
			_, _ = s.cli.Revoke(ctx, freshLease)
		}
		return false, nil
	}
	return true, nil
}

// putOpts returns the Put options that attach TTL to a write, plus the
// lease ID if this call granted a fresh one (0 if ttl <= 0, or if an
// existing lease was reused). A lease is etcd's server-side TTL handle:
// every key attached to it is deleted when the lease expires. etcd's lease
// granularity is whole seconds with a minimum of 1, so sub-second TTLs round
// up.
//
// Repeat writes to a key reuse whatever lease GetState last observed
// attached to it (see the leases field doc), refreshing its countdown with
// a single KeepAliveOnce and explicitly re-attaching it (WithLease, not
// WithIgnoreLease — the cached ID may belong to a lease another writer
// granted, so it must be named, not assumed already-attached). This is
// deliberately not KeepAlive: a continuous keepalive would keep the lease —
// and the state under it — alive forever regardless of write activity,
// which defeats expire-after-write. KeepAliveOnce resets the countdown once,
// exactly like a fresh Grant would, but without minting a new lease object.
func (s *store) putOpts(ctx context.Context, key string, ttl time.Duration) ([]clientv3.OpOption, clientv3.LeaseID, error) {
	if ttl <= 0 {
		return nil, 0, nil
	}

	if leaseID, ok := s.getLease(key); ok {
		if _, err := s.cli.KeepAliveOnce(ctx, leaseID); err == nil {
			return []clientv3.OpOption{clientv3.WithLease(leaseID)}, 0, nil
		}
		// Cached lease is gone: expired, or stale. Fall through to granting
		// fresh; the next GetState will re-observe whatever wins.
		s.clearLease(key)
	}

	seconds := int64(ttl / time.Second)
	if ttl%time.Second != 0 || seconds == 0 {
		seconds++
	}
	lease, err := s.cli.Grant(ctx, seconds)
	if err != nil {
		return nil, 0, err
	}
	return []clientv3.OpOption{clientv3.WithLease(lease.ID)}, lease.ID, nil
}

func (s *store) getLease(key string) (clientv3.LeaseID, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.leases[key]
	return id, ok
}

func (s *store) setLease(key string, id clientv3.LeaseID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leases[key] = id
}

func (s *store) clearLease(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.leases, key)
}

func (s *store) Remove(ctx context.Context, key string) error {
	if _, err := s.cli.Delete(ctx, s.key(key)); err != nil {
		return fmt.Errorf("etcdstore: remove %q: %w", key, err)
	}
	s.clearLease(key)
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
	s := &store{cli: cli, prefix: c.prefix, leases: make(map[string]clientv3.LeaseID)}
	return core.NewCASProxyManager(s, core.WithExpireAfterWrite(c.ttl))
}
