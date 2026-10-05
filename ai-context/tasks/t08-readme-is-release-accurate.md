# T08 — `README.md` is release-accurate (row 11, now a release blocker)

**For:** Dev. Prose only, no Go code. **Blocks** `publish-v0.1.0.md`.

**This is roadmap row 11**, promoted out of tier 3. The reason it was tier 3 is
that stale docs cost nothing when nobody reads them; the reason it is a blocker
now is that `README.md` is the page a stranger lands on. A first release with a
README that undersells the library by two backends is worse than a late one.
Row 11 is not duplicated — it points here.

## What is actually stale

Re-checked against `cb1766d`:

- `README.md:3` — "ported from bucket4j". `CLAUDE.md` and
  `docs/adr/0018-deliberate-deviations-from-bucket4j.md` both say *rewritten*:
  semantics carried across, not code transliterated, and the divergences are
  deliberate. "Ported" invites exactly the "is this a copy?" reading that
  `t07`'s `NOTICE` exists to close, so the two must not contradict each other.
- `README.md:5` — "swap **local ↔ Redis ↔ Postgres**". Four backends now.
- `README.md:21-22` — the commented constructor lines show only Redis and
  Postgres. Signatures to match: `redisstore.NewProxyManager(rdb redis.Cmdable, opts ...Option)`
  (`redisstore/redisstore.go:110`), `pgstore.NewProxyManager(pool *pgxpool.Pool, ...)`
  (`pgstore/pgstore.go:254`), `mongostore.NewProxyManager(coll *mongo.Collection, ...)`
  (`mongostore/mongostore.go:143`), `etcdstore.NewProxyManager(cli *clientv3.Client, ...)`
  (`etcdstore/etcdstore.go:190`). Note the snippet also writes
  `.Bucket("user:42", cfg)` on one line — check that chain against
  `core/proxymanager.go` rather than copying it forward.
- `README.md:47-54` — the Modules list names four of six; `mongostore/` and
  `etcdstore/` are missing. So is the fact that `core` ships the shared
  conformance suite (`core/storetest`), which is the thing that makes the
  swap claim verifiable rather than aspirational.
- `README.md:55-59` — Docs list links `docs/client/redis.md` and
  `postgres.md`; `docs/client/mongodb.md` and `docs/client/etcd.md` exist and
  are unlinked.
- `README.md:71` — "three backends". Four.
- `README.md:73-79` — the test block runs `core`, `redisstore`, `pgstore`.
  Missing `mongostore` and `etcdstore`. Discover the module list from
  `go.work` rather than hardcoding a fifth line that rots the next time a
  module is added.
- `ai-context/README.md:9-11` — "storage providers (local, Redis, Postgres)"
  and "all three". (`t02` already fixed the package name on line 11; the
  count is still wrong.) Line 20 of the same file lists `docs/client/` as
  "(`redis.md`, `postgres.md`)" and there are four.
- `CLAUDE.md`'s "Time source" bullet claims both clocks are "injectable via
  `WithClock` for tests". Only the distributed path is: `WithClock` is a
  `ProxyOption` (`core/proxymanager.go:31`) and `NewLocalBucket` takes no
  options at all (`core/local.go:26`) — the local path injects through the
  unexported `newLocalBucketClock` (`core/local.go:34`). Correct the sentence;
  **do not** export a local option to make the sentence true.
- `CLAUDE.md`'s TDD convention bullet points at acceptance criteria "in
  `ai-context/`" and no such file exists there — the original lived under the
  deleted `docs/internal/` (ADR 0007), so the AC ids have no registry beyond
  `grep -rn 'AC-'`. **Fix the pointer, not the gap:** say plainly that the ids
  are referenced from test names and that `grep` is the index. Building a real
  AC registry is a separate decision and is not a release blocker — a
  consumer never sees an AC id.

## Also worth saying, since this is the release README

- A `## Status` line that says what v0.1.0 *is*: four backends, single
  bandwidth per bucket, blocking consume, lockstep-versioned at `v0.1.0`
  (ADR 0019), **pre-v1.0 so the compatibility promise is weaker than Go's
  normal one** — `t09` owns the exact wording of the storage-format half of
  that; do not invent a second phrasing here, link or reuse it.
- The repo is private at v0.1.0, so no pkg.go.dev badge and no `go get`
  instructions that will 404 for a reader without access.

## Out of scope

- `docs/client/*` content, except linking to it. `t09` edits those files.
- `core`'s package doc comment — `t11`.
- Any new document (`CONTRIBUTING`, `SECURITY`, architecture overview). Ruled
  out of v0.1.0 in `next-roadmap.md`'s release note; the repo is private and
  single-author, so there is no contributor and no vulnerability reporter to
  serve yet.
- Rewriting the "One budget across every replica" demo. It is the strongest
  part of the README and it is accurate.

**Done looks like:** every count, backend name, doc link and test command in
`README.md` matches what is in the tree; the Go snippet compiles if pasted
into a file with the right imports; `grep -rn 'three backends\|local, Redis, Postgres'`
returns nothing outside `ai-context/done/` and git history.

## Test scenarios

**Black-box (tester, Gate 2):** no black-box surface — prose only. Two
mechanical checks worth running because they are what actually rots:
`e2e/k6/run-sandbox.sh redis` once as a no-regression pass (5 accepted, 3
rejected), and the README's own console transcript at `README.md:30-42`
followed literally against `examples/redis` — if the output no longer matches,
the README is still wrong after this ticket.

**Unit / integration (Dev):** none, and no AC id — documentation accuracy is
not a behavioural criterion. The one place a test is warranted is the Go
snippet, and `t11` covers that properly by turning the equivalent code into a
compiled, run-asserted `Example` in `core`. If the README snippet and `t11`'s
example drift, the example is the one that is checked.
