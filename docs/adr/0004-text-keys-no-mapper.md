# Bucket keys are stored as TEXT, no BIGINT/VARCHAR mapping table

pgstore's schema stores the bucket key as `TEXT` directly. We considered and
rejected a mapping table (VARCHAR key → BIGINT surrogate id, used everywhere
else) — the usual RDBMS instinct for keeping join/index columns narrow. A
mapper adds a second round trip (or a join) on every operation and a second
place state can go stale, to save index width on a column that is not, in
practice, huge. Given the store is a dumb byte-blob KV and the key is opaque
application data, the simpler direct-TEXT schema was chosen; revisit only if
key-column size is measured to matter.
