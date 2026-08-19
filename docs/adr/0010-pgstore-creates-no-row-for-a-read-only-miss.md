# pgstore creates no row for a read-only miss: insert inside the locking transaction

`SELECT ... FOR UPDATE` cannot lock a row that does not exist, so `Store.doOnce`
handled a missing key by inserting a placeholder (`state` NULL, `expires_at`
NULL), **committing it**, and returning `done=false` so `Do` would retry and
lock it on the next round. The commit is what makes the row visible to the
retry — and what makes it permanent. For a key that is only ever read
(`AvailableTokens`, `EstimateAbilityToConsume`), the callback returns
`write=false`, nothing ever sets `expires_at`, and `RemoveExpired`'s
`expires_at IS NOT NULL` predicate can never reclaim the row. Redis and
memstore create nothing on the same path: an unbounded, backend-visible
divergence in storage footprint, from an operation the caller reasonably
believes is a read.

**Ruled:** the insert moves *inside* the transaction that runs the callback,
and happens *after* it, only when the callback asks to write.
`SELECT ... FOR UPDATE` → no rows → run `fn(nil, false)` → if `write`,
`INSERT ... (id, state, expires_at) ... ON CONFLICT DO NOTHING`; if
`RowsAffected() == 0` another writer created the key first, so discard and
retry the whole transaction (its `SELECT ... FOR UPDATE` now finds the row).
If `write` is false, no statement runs at all and the transaction rolls back.
A read of an absent key touches nothing, which is exactly what Redis does.

The correctness argument is that the callback's result is never applied on top
of a state it did not read. A successful insert proves no committed row existed
when we inserted (we hold the new tuple exclusively until commit; a concurrent
`Do` on the same key blocks on the unique index and then re-reads). A conflict
proves one did, and that path throws the computed state away rather than
writing it. The pre-existing `maxLockRetries` loop stays, but it now bounds
create-race losses instead of being the normal first-touch path — the common
case drops from two transactions to one.

**Rejected: give the placeholder a short `expires_at` so the sweep reclaims
it.** It is not a fix when `WithTTL` is unset (`ttl == 0` means no expiry, the
default), which is the configuration where the leak is unbounded — the leak
would survive precisely where it matters. It also keeps writing to the database
on a read.

**Rejected: probe for existence and skip the insert when the key is absent and
the operation is read-only.** The store does not know an operation is read-only
until the callback has already run, so this is the ruled fix with an extra
round trip and a race window bolted on.

**Accepted cost.** A lost create race writes a heap tuple that is immediately
super-deleted, leaving a dead tuple for autovacuum. This is bounded by the
number of concurrent first touches of a given key, versus a permanent live row
per read-only key, and it does not occur on the steady-state path where the row
already exists. Rows leaked by the previous behaviour are not migrated: they
are identifiable as `state IS NULL` and a one-line `DELETE` is documented in
`docs/client/postgres.md` rather than added to the sweep, which would trade its
partial index for a sequential scan on every run. `doOnce` keeps treating a
NULL `state` as absent so such a row still reads as a fresh bucket.

Covered by AC-48 (`TestAC48_...` in `pgstore/pgstore_test.go`): a read-only
operation against a never-written key leaves the bucket table empty, asserted
against a real Postgres container per ADR 0006.
