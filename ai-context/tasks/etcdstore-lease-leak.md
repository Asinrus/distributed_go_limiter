# etcdstore grants a lease per write attempt and revokes none

**Symptom:** every `CompareAndSwap` call grants a fresh etcd lease before it
knows whether the transaction will succeed. Lost CAS races therefore leave a
lease attached to nothing, and a lease displaced by a successful overwrite is
left to expire on its own. Live lease count grows with *write attempts in the
last TTL window*, not with live keys.

**Evidence**
- `etcdstore/etcdstore.go:39` — `putOpts` is called before the `Txn` is built,
  unconditionally.
- `etcdstore/etcdstore.go:75` — `cli.Grant`, one per call.
- `etcdstore/etcdstore.go:51-58` — the `Txn` may return `Succeeded == false`;
  nothing revokes the lease granted for the attempt that lost.
- `examples/etcd/main.go:36` — `WithTTL(time.Hour)`, the value a reader is
  likeliest to copy.
- Half the behavior is documented as deliberate: `etcdstore/etcdstore.go:61-66`
  and `docs/client/etcd.md:22-24` both describe the lease displaced by an
  overwrite being left to expire. This task is the argument that it does not
  survive contact with real traffic.
- The other half is not documented, it is *contradicted*.
  `docs/client/etcd.md:22` says "Every **successful** write grants a fresh
  lease"; the code grants at `etcdstore/etcdstore.go:39`, before the `Txn` at
  `:51` decides anything, so a losing attempt grants a lease too. That sentence
  is wrong today and must be corrected whichever fix is chosen — including the
  do-nothing one.

**Why it matters under load.** At 100 requests/second with the example's 1h
TTL, roughly 360,000 leases are live at steady state, every one of them a
server-side object etcd tracks and checks. Lease bookkeeping is not free and
this is a monotonic climb until the first writes age out an hour later. The
second cost is on the request path: every token check is **two** round trips
(`Grant`, then `Txn`) where the other CAS backends need one, and the extra one
happens even for the attempt that is about to lose its race.

**Constraint the fix must respect.** ADR 0012 classifies etcd as a backend
whose TTL *is* a correctness mechanism: the key is deleted on time, so a read
after expiry cannot observe stale state and the store carries no expiry logic.
Any approach that makes expiry coarse (a shared time-bucketed lease) or moves
it into an application-level predicate changes that classification and needs
ADR 0012 amended in the same change — it must not be done silently.

Directions worth pricing, with the obvious objection to each stated rather than
discovered later: revoke the lease when the `Txn` reports `Succeeded == false`
(cheapest, fixes only the lost-race half, still two RPCs); reuse a lease per key
and refresh it on write instead of granting a new one (bounds live leases by
live keys, but a refreshed lease must still be *reset*, not extended
indefinitely — `KeepAlive` would make state immortal, which is why it was
avoided); a shared lease per TTL window (one RPC amortized, but coarse expiry,
so see the ADR 0012 constraint). Pick one, and record the rejected ones with
their reasons — this is the kind of decision that gets reopened.

**In scope**
- `etcdstore` only. No change to `core.CASStore`, and no change to the other
  three backends.
- `docs/client/etcd.md:21-27` updated to describe whatever the new behavior is,
  including the observable TTL granularity if it changes.

**Out of scope**
- Reducing the request path below one round trip; batching; any change to the
  `WithTTL` public signature.

**Done looks like:** a sustained run of writes against a real etcd container
leaves a lease count bounded by live keys rather than by write attempts, the
TCK still passes, expiry still happens within the documented window, and the
choice is recorded with its rejected alternatives.

## Test scenarios

**Black-box (tester, Gate 2):** `e2e/k6/run-sandbox.sh etcd` exits 0 with the
existing contract unchanged — 5 accepted, 3 rejected in the burst scenario, and
the refill scenario's checks all passing. Lease counts are not observable
through HTTP and the tester does not query etcd (ADR 0011); its job here is to
prove the fix cost no rate-limiting behavior. Also run
`e2e/k6/run-sandbox.sh redis` as the control, since a behavior difference
between the two would point at `core` rather than at this change.

**Unit / integration (Dev):**
- After N successful writes to a single key with a TTL set, the number of live
  leases in etcd is bounded (by live keys, or by the chosen scheme's constant) —
  not proportional to N. This is the assertion that fails today.
- A lost CAS race — a `CompareAndSwap` whose expected bytes no longer match —
  leaves no lease behind.
- Expiry still works: a key written with a short TTL is gone from `GetState`
  within the documented window, and the surrounding wait is not longer than the
  existing `etcdstore/etcdstore_test.go:132` sleep unless the granularity
  deliberately changed.
- The TCK (`tck.Run`) still passes against a real etcd container.
