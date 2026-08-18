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
