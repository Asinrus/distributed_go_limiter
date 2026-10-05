# T01 — Record the owner's rulings

**Blocked by:** none. This is the head of the T01→T05 chain; T02 and T03 both
wait on it because they build on decisions ratified here.

**For:** the Architect. This ticket is documentation only — no Go source is
touched, and the work described here is already committed on
`asinrus/t01-record-rulings`. It exists so the chain has a head with a
verifiable "done", and so a reader of `ai-context/tasks/` can see which
rulings the other four tickets rest on.

## The rulings

1. **The blocking API stays.** Confirmed scope from the initial commit
   ("Scope: lean + blocking"), specified by AC-31/AC-32 — whose only surviving
   statement is the comments at `core/blocking_test.go:12,27,45`, since the AC
   registry was never committed. Nothing to change; recorded so the surface is
   not re-litigated as scope creep.
2. **No read ever exposes a negative token count (AC-53).** ADR 0018.
3. **A cancelled blocking reservation is refunded (AC-54).** ADR 0018.
4. **No backend tiers.** The local path is the main one; every store under
   `ProxyManager` is a peer. Go modules are independent, so a backend nobody
   imports costs a consumer nothing. The earlier "core backend / reference
   backend" tier idea is **withdrawn** and is deliberately not recorded
   anywhere as a decision.
5. **etcd stays**, as a peer of Redis/Postgres/Mongo. Its only cost is CI
   upkeep, which is not a reason to delete working code — accepting that a
   `govulncheck` failure in any module fails the whole workspace. Recorded in
   `ai-context/done/etcd-as-a-main-backend.md`.
6. **Hazelcast: not building it now.** Reversible, so no ADR. Recorded in
   `ai-context/done/vendor-hazelcast-store.md`.
7. **`core/tck` → `core/storetest`.** See T02.
8. **Vocabulary:** *vendor* / *store* / *backend*, with "provider" retired
   except inside the frozen provider-swap / AC-46 phrase. Recorded in
   `CLAUDE.md`.
9. **This is a rewrite, not a port** (added in review of the PR for this
   ticket). Java and Go do not map one-to-one either way, so intent and
   observable behaviour are what carry across, not structure; where a Java
   construct has no good Go expression the Go expression wins, and sometimes no
   faithful equivalent exists at all. A difference from upstream is therefore
   not by itself evidence the Go is wrong — each stands on its own reasoning.
   Recorded as the premise of ADR 0018 and in `CLAUDE.md`. **No separate ADR:**
   it reverses no earlier decision and is stated where a reader of either file
   already lands, which a third file would not improve.

## What was produced

- `docs/adr/0018-deliberate-deviations-from-bucket4j.md` — rulings 2 and 3
  together, with the upstream citations, framed by ruling 9. This is the
  load-bearing artifact: both are one-line changes with a plausible-looking
  upstream source arguing the opposite, and without the ADR the predictable
  failure is a contributor reading `BlockingBucket.java:47-51` and reverting the
  refund in the belief that matching Java is the goal. The filename keeps
  "deviations" although the H1 no longer does — the number is the stable id and
  three files link to the path.
- `CLAUDE.md` — the ruling-8 glossary, and the module count corrected from 4 to
  6 (`mongostore/` and `etcdstore/` were missing from the layout list
  entirely).
- `ai-context/done/` — `vendor-hazelcast-store.md` (ruling 6),
  `etcd-as-a-main-backend.md` (ruling 5, rescued from an unmerged branch), plus
  `github-actions-ci.md` and `grpc-vuln-bump.md`, whose PRs merged long ago and
  whose files were never moved.
- `ai-context/tasks/next-roadmap.md` — rulings 4-6 applied.
- T02 … T05 in this folder.

**No fifth doc file.** A standalone glossary document was considered and
rejected: three definitions do not earn a file, and `CLAUDE.md` is already the
first thing every role reads.

**Ownership note.** `ai-context/README.md` lists `CLAUDE.md` as Dev-owned and
tells the Architect to open a task rather than edit it silently. The edits here
were directed by the owner and are process/vocabulary, not code map — this file
is the record that they were not silent. The `core/tck` references inside
`CLAUDE.md` were deliberately left alone for T02 to change with the rename,
which it did.

## Test scenarios

**Black-box (tester, Gate 2):** none — no black-box surface. This ticket
changes only markdown under `docs/adr/`, `ai-context/` and `CLAUDE.md`. Run
`e2e/k6/run-sandbox.sh redis` once as a no-regression check that the tree still
builds and the existing contract holds (5 accepted, 3 rejected; refill checks
pass).

**Unit / integration (Dev):** none — no Go code changes. The checkable
properties are documentary and belong to review, not to a test binary:
- ADR 0018 is the next free number, checked against every remote branch and not
  just `main`.
- Every `path:line` in the new and edited files resolves in the tree it cites
  (repo paths against this branch; `BucketState64BitsInteger.java`,
  `BlockingBucket.java`, `AbstractBucket.java` and
  `BlockingConsumeSpecification.groovy` against upstream bucket4j).
- Nothing under `core/`, `redisstore/`, `pgstore/`, `mongostore/`,
  `etcdstore/` or `examples/` is modified.

**Done looks like:** every one of the rulings is discoverable from
`docs/adr/` or `ai-context/` without asking the owner again, and the two
rulings that contradict upstream carry the upstream citation that would
otherwise reopen them.
