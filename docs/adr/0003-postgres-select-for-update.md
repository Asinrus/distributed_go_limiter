# Postgres v1 uses `SELECT ... FOR UPDATE`, not advisory locks

pgstore implements `core.LockStore` via a row lock (`SELECT ... FOR UPDATE`)
rather than `pg_advisory_lock`. Advisory locks avoid a physical row and its
index maintenance, but they key on a 64-bit integer, not the string bucket
key, so a v1 built on them would need a hashing/mapping layer up front — the
same BIGINT/VARCHAR-mapping shape we separately rejected for the schema
(see the TEXT-key ADR). Row locking needs no such layer and matches the
existing `bucket4j-postgresql` reference implementation. Advisory locks are
deferred, not ruled out, if row-lock contention becomes a measured problem.
