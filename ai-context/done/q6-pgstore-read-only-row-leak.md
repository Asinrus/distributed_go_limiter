# Q6 — pgstore leaks a permanent row for read-only keys

**Origin:** `SELECT ... FOR UPDATE` needs a row to lock, so pgstore commits a
placeholder row (`expires_at` NULL) before a read-only callback
(`AvailableTokens`/`EstimateAbilityToConsume`) runs, for any key that is only
ever read, never written. The sweep can never reclaim it because
`expires_at` stays NULL. Redis/memstore create nothing on the same path — an
observable divergence between backends. No AC covers storage footprint; this
is a spec gap, not a defect.

**Goal:** Architect picks a fix. Candidates (from the prior investigation,
not re-derived here — re-derive if this task is picked up without that
context available):
1. Skip the row-insert on genuinely read-only operations where the key
   doesn't already exist — accept a race window where a concurrent write
   between the existence check and insert has to retry.
2. Give the placeholder row a short `expires_at` (not NULL) so the existing
   sweep reclaims it even without ever being written — cheap, but a read-only
   key would then "expire" and reset even under sustained read traffic.

**Acceptance:** Ruling recorded (ADR — this is a real backend-parity
trade-off). Dev implements it; a test proving the chosen backend no longer
leaks (or bounds the leak) passes, run against a real Postgres container.

## Ruling — insert moves inside the locking transaction, after the callback

Confirmed against the code: `doOnce` inserts `(id, NULL)` and **commits** on
`pgx.ErrNoRows` so the retry has a row to lock; `RemoveExpired` filters on
`expires_at IS NOT NULL`, so a key that is only ever read keeps that row
forever. Real, and unbounded whenever `WithTTL` is unset.

Recorded in `docs/adr/0010-pgstore-creates-no-row-for-a-read-only-miss.md`.
Neither candidate was taken:

1. **Existence-probe-then-skip: rejected.** The store cannot know an operation
   is read-only until the callback has run, so the probe is the chosen fix plus
   an extra round trip and a race window.
2. **Short `expires_at` on the placeholder: rejected.** `ttl == 0` (the
   default, no expiry) is exactly the configuration where the leak is
   unbounded, and that is the configuration where this does nothing. It also
   keeps writing to the database on a read.

**Chosen:** `SELECT ... FOR UPDATE` → no rows → run the callback in the *same*
transaction → `write=false` issues no statement and rolls back (nothing
persisted, parity with Redis/memstore); `write=true` does
`INSERT ... ON CONFLICT DO NOTHING`, and `RowsAffected() == 0` means another
writer created the key first, so the computed state — derived from a state we
never read — is discarded and the existing `Do` loop retries. The retry loop
now bounds create-race losses instead of being the normal first-touch path.

**Follow-up (implementation):**
`ai-context/tasks/pgstore-no-row-for-read-only-miss-ac48.md` — the code change,
AC-48, and the client-doc note (including the `DELETE ... WHERE state IS NULL`
cleanup for tables written by the old build). That task carries the `/verify`
gate; this file needs no code change of its own.
