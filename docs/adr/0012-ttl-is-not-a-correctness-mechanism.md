# A backend's TTL feature is only a correctness mechanism if the backend deletes on time

`redisstore` and `etcdstore` delegate expiry to the backend (Redis expiry,
etcd leases): both remove the key at expiry, so a read after expiry cannot
observe stale state and the store code carries no expiry logic.

`pgstore` and `mongostore` cannot. Postgres has no expiry at all, and
MongoDB's TTL index is reclaimed by a background reaper that runs roughly once
a minute — an expired document stays readable for up to that long. So both
store an explicit `expires_at`/`expiresAt` and filter on it at read time;
MongoDB's TTL index is kept purely to reclaim storage, and `pgstore` exposes
`RemoveExpired` for the same job.

The consequence that is easy to get wrong: in these two backends,
create-if-absent must match "absent **or** expired", not "absent". A plain
insert against a not-yet-reaped expired row/document fails forever, and the
`CASStore` executor — which was told `found=false` and therefore passes
`old=nil` — would retry into that failure indefinitely. `pgstore` avoids it by
treating an expired row as `found=false` under the row lock and updating it;
`mongostore` must use an upsert whose filter is the negation of its live
predicate.

Rejected: adding an expiry-aware primitive to `core.CASStore`/`core.LockStore`
so the executor could distinguish "absent" from "expired". It would push a
backend limitation into the contract every backend implements, to save two
backends a predicate they already need for reads.
