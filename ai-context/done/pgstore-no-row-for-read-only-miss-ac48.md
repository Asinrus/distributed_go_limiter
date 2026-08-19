# pgstore — no row for a read-only miss, and give it AC-48

**Origin:** Q6 ruling, `docs/adr/0010-pgstore-creates-no-row-for-a-read-only-miss.md`.
Replaces `ai-context/tasks/q6-pgstore-read-only-row-leak.md` (that file is the
question; this is the implementation). Behaviour change in `pgstore` only — no
`core`, `redisstore`, or `examples` change, and no schema change.

**Goal:**

1. **`Store.doOnce` (`pgstore/pgstore.go`): stop committing a placeholder row.**
   On `pgx.ErrNoRows` from `selectSQL`, do *not* insert-and-commit and do *not*
   return `done=false`. Instead run the callback with `fn(nil, false)` in the
   same transaction, and only then:
   - `write == false` → no statement at all; return `done=true` and let the
     deferred rollback release the transaction. Nothing is persisted.
   - `write == true` → `INSERT` the state (with the computed `expires_at`)
     `ON CONFLICT (<idCol>) DO NOTHING`. `RowsAffected() == 1` → commit,
     `done=true`. `RowsAffected() == 0` → another writer created the key first;
     the computed state was derived from a state we never read, so discard it,
     return `done=false`, and let the existing `Do` loop retry (its
     `SELECT ... FOR UPDATE` will find the row). Do **not** fall through to
     `updateSQL` here.

   `insertSQL` needs to carry state and `expires_at` now:
   `INSERT INTO t (id, state, expires_at) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING`.
   The existing-row branch is unchanged apart from skipping the pointless
   `Commit` when `write == false` (rollback is equivalent and cheaper). The
   `expires_at` computation (`ttl > 0` → `now()+ttl`, else NULL) is shared by
   both branches — factor it into one small helper rather than duplicating it.

   Keep `found := state != nil`: rows leaked by the old behaviour have a NULL
   state and must keep reading as a fresh bucket, not as a decode error. Say so
   in one line where the check lives; the `maxLockRetries` comment ("placeholder
   row was inserted; retry so it can be locked") is now wrong and must say the
   retry is for a lost create race.

2. **AC-48.** Storage footprint had no acceptance criterion. AC-48 = *a
   read-only operation on a key that was never written persists no row; only a
   write creates one.* New test in `pgstore/pgstore_test.go`, named
   `TestAC48_ReadOnlyMissPersistsNoRow`, against a real Postgres container per
   `docs/adr/0006-testcontainers-for-backend-tck.md`:
   - `AvailableTokens` then `EstimateAbilityToConsume` on a fresh key, with no
     TTL configured (the `ttl == 0` case is the one where the old leak was
     unbounded); both must return the fresh-bucket values they return today.
   - `SELECT count(*) FROM bucket` is **0** afterwards.
   - Then `TryConsume` on that same key → count is 1, and a subsequent
     `AvailableTokens` still reflects the consumption (proves the read path did
     not break the write path).

3. **`docs/client/postgres.md`**: two short additions. (a) A read of a key that
   was never written stores nothing, so `RemoveExpired` has nothing to reclaim —
   footprint tracks writes, not reads. (b) A table written by an earlier build
   may hold leaked placeholder rows; they are the rows with a NULL state and
   `DELETE FROM bucket WHERE state IS NULL;` clears them. Deliberately not in
   `RemoveExpired` — see the ADR.

**Independence:** self-contained in `pgstore` + `docs/client`. Do not touch
`RemoveExpired`'s query.

**Acceptance:** `/verify` green; the `pgstore` suite (TCK AC-41..44, AC-45,
AC-47 and the new AC-48) passes against a real Postgres container.
`AC43_AtomicInit` is the regression guard for the create-race path — it must
stay green, and if it ever exhausts `maxLockRetries` the retry logic is wrong,
not the limit.
