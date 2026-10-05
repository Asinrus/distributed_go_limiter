# Next roadmap — what is in flight

The one place that answers "what is queued and in what order". The **Tier
column is the ranking** — `now` before tier 1, tier 1 before tier 2, and so on.
The `#` column is a stable id, not a position: a re-rank moves tiers and leaves
every cross-reference to "row 7" pointing at the same thing. Ranked, not
scheduled. A row with its own task file links to it; a row without gets one
written when it is picked up — the row is the placeholder, not the spec.

**Re-ranked after the repo owner answered four direction questions.** The
answers, since they explain tiers that would otherwise look arbitrary:

1. This is a **portfolio / reference implementation**, not a near-term public
   release. Row 15 is dormant.
2. **Multi-bandwidth is planned for v0.2**, so the codec format changes anyway
   — row 9 folded into row 17 rather than breaking the format twice.
3. **Cheap pluggability is the product.** Rows 7 and 8 are what make the swap
   guarantee real, so they are tier 1; and `core` stays vendor-agnostic, which
   is row 19.
4. `examples/` are demonstrations that a reader will nonetheless copy from, so
   row 14 is tier 2.

This file is an **index, not a unit of work**: it never moves to
`ai-context/done/`, and the Test scenarios requirement
(`docs/adr/0013-architect-scopes-test-coverage.md`) applies to the rows' own
task files, not to this one. Whether a queue index belongs here at all was
the open question in `ai-context/tasks/plans-md-vs-changelog.md`; it is now
ruled (`docs/adr/0016-retire-plans-md.md`) — `PLANS.md` is deleted and this
file is confirmed as the standing queue index.

**Eight further rulings landed with T01** (`t01-record-rulings.md`), and four
of them change rows here:

- **No backend tiers.** The local path is the main one; every store under
  `ProxyManager` is a peer. A backend nobody imports costs a consumer nothing,
  because Go modules are independent. The "core backend / reference backend"
  idea floated while row 20 was deferred is **withdrawn** — it is not a
  deferred option, it is not an option.
- **etcd stays**, as a peer (`ai-context/done/etcd-as-a-main-backend.md`). Its
  cost is CI upkeep, accepted. This un-defers row 20.
- **Hazelcast is not being built now** — reversible, no ADR, filed to
  `ai-context/done/vendor-hazelcast-store.md`.
- **AC-53 and AC-54 are decided differences from bucket4j**, not bug fixes
  (`docs/adr/0018-deliberate-deviations-from-bucket4j.md`). Rows 21 and 22 are
  now one ticket, `t03-reservation-semantics.md`.

**Owner ruling (1) is reversed — v0.1.0 is being prepared.** This repo is no
longer "a portfolio implementation with no near-term release". Four decisions
came with the reversal, ratified in
`docs/adr/0019-v0-1-0-is-five-lockstep-modules-under-apache-2-0.md`: Apache-2.0
in all seven roots, lockstep `v0.1.0` across the five library modules
(`examples` untagged), PRs #25–#30 land first, and the repo **stays private** —
so v0.1.0 is *tagged but unpublished* and pkg.go.dev indexes nothing until
visibility flips. What this does to the ranking:

- Row 15 is **no longer dormant** and is the last step of the release, not the
  first — the tags must point at a commit where everything else has landed.
- Rows 11 and 18 are **release blockers, not tier-3 cleanup.** Stale docs cost
  nothing while nobody reads them; `README.md` is the page a stranger lands on,
  and row 18's promise has to exist *before* the v0.2 format break, not with
  it. Both now have tickets (`t08`, `t09`).
- Rows 24, 25 and 26 are new: license, changelog, and the pkg.go.dev landing
  surface. Row 26 is the gap that was invisible from inside the repo — a
  module in a subdirectory shows its package doc, not `README.md`.
- Nothing else is re-ranked. The release does not wait on rows 10, 13, 14, 19,
  20 or 23, and shipping 0.1.0 with known tier-2 defects recorded here is the
  point of calling it 0.1.0.

The `path:line` references are the evidence for each row. Rows 11, 15, 18, 24,
25 and 26 were re-verified against `890f6b1` with the release scoping, and rows
7, 20 and 22 were re-pointed by T02b/T02c; the rest still read against
`20de7dc` / `5f45eb8` and have not been re-confirmed since. If one stops
resolving, the row is stale and the finding needs re-confirming before anyone
works it.

| # | Task | What it is | Tier |
|---|------|-----------|------|
| 1 | _(done)_ Go 1.23 pin (`go-version-1-23.md`) | One version across `go.work`, six `go.mod`, CI and docs, plus the ADR | — |
| 2 | _(done)_ GitHub Actions CI (`github-actions-ci.md`) | Module-discovering workflow at Go 1.23: race, vet, gofmt, govulncheck | — |
| 3 | _(done)_ `initialTokens` validation (`core-initial-tokens-validation.md`) | `core/bandwidth.go:50,59` — `valid()` never checks it; `n > capacity` is accepted and `core/state.go:61`'s early return means refill never clamps, so the over-capacity burst is permanent; negatives accepted too | — |
| 4 | _(done)_ etcd lease leak (`etcdstore-lease-leak.md`) | `etcdstore/etcdstore.go:39,75` — `cli.Grant` before every `Txn` including lost CAS races, leases never revoked → ~360k live leases at 100rps with the example's 1h TTL, plus 2 RPCs per token check | — |
| 5 | _(done)_ CAS retry hardening (`core-cas-retry-hardening.md`) | `core/command.go:24,45` — 100 retries with no backoff and no jitter. Kept separate from row 6 (ruling in both files): pacing has no AC and an open timer question, cancellation is a contract with two. Row 6 lands first and settles the one shared line — the backoff sleep is `ctx.Done()`-interruptible and returns the bare context error | — |
| 6 | _(done)_ Honor ctx cancellation (`core-ctx-cancellation.md`) | `core/memstore.go:43` + `core/command.go:45` — ctx discarded end to end; a cancelled ctx still consumes a token and returns success, contradicting `core/bucket.go:29`. Error shape now **decided**, not deferred: bare context error, `storeErr` (`core/command.go:17`) as the choke point **Ruled** (ADR 0015, ratified by the owner). PR #12 implements the opposite (`ErrStore`-wrapped) and must be changed to `return ctx.Err()`, and AC-51 clause 1 needs the carve-out — both applied in those PRs, not here | — |
| 7 | _(done)_ Widen the conformance suite (`t05-widen-conformance-suite.md`) | `core/storetest/storetest.go`'s `Run` runs only AC-41..44 — probes, estimate, `TryConsumeAsMuchAsPossible` and blocking `Consume` never run against a real backend. Tier 1 by owner ruling (3): the suite *is* the provider-swap guarantee `README.md` advertises, and four cases do not carry it. Scoped as **AC-56..AC-59**; blocked by rows 21/22 (T03) and row 8 (T04) | — |
| 8 | _(done)_ `ProxyOption` on backends (`t04-proxyoption-reaches-backends.md`) | `redisstore/redisstore.go:110`, `pgstore/pgstore.go:254`, `mongostore/mongostore.go:143`, `etcdstore/etcdstore.go:190` all take their own `Option` and none forwards `core.ProxyOption`, so `core.WithClock` (`core/proxymanager.go:31`) is unreachable. Tier 1 by owner ruling (3): a backend author cannot reach a core option, which is a pluggability defect, not an ergonomics one. **Correction:** this row previously claimed it would end the 3s sleep in the etcd expiry test — it will not. `WithClock` is the *refill* clock; expiry is the vendor's (etcd lease, Redis `EXPIRE`) or an unexported store-side clock (`pgstore/pgstore.go:129`, `mongostore/mongostore.go:50`). What it actually buys is refill-driven assertions against a real backend, which is why it blocks row 7 | — |
| 9 | _(merged into row 17)_ | Config fingerprint — `core/proxymanager.go:56-60`, config is not persisted and the codec carries none, so two nodes with mismatched config on one key silently enforce different limits. Real, but it needs a codec format change and so does multi-bandwidth; doing them separately breaks the format twice | later |
| 10 | memstore expiry sweeper | `core/memstore.go:36` — expiry is lazy-on-access only, so an untouched key is never reclaimed; unbounded memory under the per-IP keying the examples demonstrate | 2 |
| 11 | Refresh stale docs (`t08-readme-is-release-accurate.md`) | **Release blocker now, not tier 3** — stale docs cost nothing while nobody reads them, and `README.md` is the page a stranger lands on. Scoped into T08, which carries the full list. What is **still** stale, re-verified against `890f6b1`: `README.md`'s opening claim ("ported from" bucket4j, and "local ↔ Redis ↔ Postgres"), its commented constructor snippet (Redis and Postgres only), its Modules and Docs lists (no `mongostore`/`etcdstore`, no links to `docs/client/mongodb.md` or `etcd.md`), its Status line (`README.md:71` still says "three backends"; there are four) and its test block (three modules of six, and it still says "in-memory TCK" after T02); `ai-context/README.md:9` ("local, Redis, Postgres") and `:11` ("all three"); `CLAUDE.md`'s TDD convention bullet, which points at acceptance criteria "in `ai-context/`" — no such file exists there (the original lived under the deleted `docs/internal/`, ADR 0007), so the AC ids have no registry beyond `grep -rn 'AC-'`; and one claim T08 added — `CLAUDE.md`'s "Time source" bullet says both clocks are `WithClock`-injectable and only the distributed one is (`core/local.go:26,34`). **Already fixed, do not re-do:** `CLAUDE.md`'s module count and its missing `mongostore`/`etcdstore` bullets (T01), its `PLANS.md` pointer (gone on `main` before T01), and `ai-context/README.md:11`'s package name (T02). Cited by section rather than line: this row's line numbers went stale twice, once from its own subject matter | now |
| 12 | Fix dead paths | `ai-context/done/tester-e2e-sandbox-harness.md:13,24,45,46` points at `examples/k6/`; the harness lives at `e2e/k6/` (the other citation, `ai-context/PLANS.md`, is moot — that file is now deleted, `docs/adr/0016-retire-plans-md.md`) | 3 |
| 13 | Greedy refill `ceil` | `core/state.go:158` truncates the division where it needs to round up, so a client that sleeps exactly the advertised wait is rejected again. The intervally branch two lines up (`core/state.go:138-140`) rounds up explicitly, which is the asymmetry that makes this look like a bug — but `core/state.go:6` calls the whole state a direct reduction of `BucketState64BitsInteger` (and `delayNanosForConsume`'s own doc at `core/state.go:111-113` claims nothing either way), so whoever picks this up establishes what bucket4j does *before* changing it; a decided difference from bucket4j is a different task than a fix | 3 |
| 14 | Example hardening | All four `examples/*/main.go` echo `err.Error()` into the 503 store-failure body (`examples/redis/main.go:44`), and every backend formats the bucket key into that error (`etcdstore/etcdstore.go:28` and siblings), so the client's own IP comes back to it along with the internal failure. No `Retry-After` on the 429 either, though `TryConsumeAndReturnRemaining` already has the wait | 2 |
| 15 | Publish v0.1.0 (`publish-v0.1.0.md`) | **No longer dormant — ruling (1) reversed.** Now the *tagging* ticket and the last step of the release: drop the `replace` lines, require `core v0.1.0`, cut five `*/v0.1.0` tags. Blocker (c) is the latent defect `go.work` hides (`redisstore/go.mod:6,65` and siblings) — `AC-61`. Three commits with a tag round between them; it cannot be one commit. Blocked by rows 11, 18, 24, 25 and 26; PRs #25–#30 have all landed (`890f6b1`). Blocker (a) (private) is deferred, so the release is *tagged but unpublished* | now |
| 16 | _(closed)_ | No open questions remain in `ai-context/tasks/`. `plans-md-vs-changelog.md` ruled (ADR 0016), `q5-pgstore-identifier-behavior-change.md` and `q9-errstore-contract-ac.md` in `done/`, `vendor-hazelcast-store.md` answered by ruling (6) and moved | — |
| 17 | Codec format change: multi-bandwidth + config fingerprint | Planned for **v0.2**. One format break carries both — the multi-bandwidth layout (N states instead of one, per ADR 0001, which deferred it from v1 and is not contradicted by planning it for v0.2) and the config fingerprint from row 9. Needs its own task file and probably an ADR before anyone writes code | later |
| 18 | Say the codec layout is unstable (`t09-codec-layout-is-unstable.md`) | **Release blocker now, not tier 3** — a v0.1.0 that stays silent makes row 17's v0.2 format break a breaking surprise. Nothing states that `core/codec.go:8-15`'s byte layout may change before v1.0. **Correction to this row's own claim:** only `docs/client/redis.md:54` and `etcd.md:41` have a "What is stored" section; `postgres.md:28` and `mongodb.md:22` call it "Schema". All four get the promise, under whichever heading they already use. The format itself is already pinned by `core/codec_test.go:44`, so no new test — the tripwire exists, it just does not say where the promise lives | now |
| 19 | Cut store round-trips without vendor code | Owner constraint, ruling (3): `core` stays vendor-agnostic — no per-vendor knowledge, no backend-specific fast paths. The way to reduce calls is the command layer (`core/command.go`), so every backend inherits it; bucket4j's optimization controllers (batching, prediction, delayed writes) are the reference. **Unscoped** — `CLAUDE.md` lists optimization/batching as deferred from v1 (ADR 0001), and this needs Architect scoping and a task file before anyone implements it | later |
| 20 | Vendor ctx conformance (`vendor-ctx-conformance.md`) | **Un-deferred** — the etcd question is answered (it stays, as a peer; `ai-context/done/etcd-as-a-main-backend.md`), so this covers four backends. Filed as a bug hunt; the premise was false — all four drivers already return `errors.Is(err, context.Canceled)`, etcd included (`ContextError`, `client.go:578-598`). Reframed as a regression guard against a driver upgrade or a fifth backend dropping the property. Still real: nothing in the repo exercises a cancelled ctx against a real backend | 2 |
| 23 | pgstore lock retry pacing (`t06-pgstore-lock-retry-pacing.md`) | `pgstore/pgstore.go:88,92` — `maxLockRetries = 16` in a tight loop with no backoff, no jitter and no ctx check between attempts. The CAS path got exactly this treatment (PR #17 / AC-55) and the `LockStore` side never did; 16 is the budget `core/command.go`'s own measurement rejected. **AC-60.** The design question is **ruled** — pacing lives in pgstore as pgstore configuration, `core`'s surface stays closed, `LockStore.Do` keeps its single-`error` signature. Queued as a follow-up; the owner chose not to build it this round | 2 |
| 22 | _(done)_ Refund a cancelled blocking reservation (`t03-reservation-semantics.md`, spec in `ai-context/done/core-blocking-reservation-refund.md`) | `core/ops.go:90` deducts before the sleep and the write is persisted, so a cancel at `core/blocking.go:54-55` leaves the tokens spent for a request nobody was served — `core/blocking.go:22-23` documents that as intended and the owner has ruled it is not. Compensating release through the executor on the cancel branch, best-effort but reported (AC-54). Now framed as a **decided difference** from bucket4j, not a bug fix — upstream never refunds, on purpose (ADR 0018) | — |
| 24 | Apache-2.0 license (`t07-apache-2-0-license.md`) | Blocker (b) of the release, decided: Apache-2.0 in the repo root and all six module roots, because a Go module is fetched on its own and pkg.go.dev renders nothing for a module whose license it cannot recognise (`gh repo view --json licenseInfo` → `null` today). Plus one `NOTICE`, root only — bucket4j ships no `NOTICE` (checked), so §4(d) propagates nothing and the file is voluntary attribution, not compliance. ADR 0019 | now |
| 25 | `CHANGELOG.md` at v0.1.0 (`t10-changelog-starts-at-v0-1-0.md`) | ADR 0016 retired `PLANS.md` with "no replacement changelog file" — about *internal tracking*, and it rejected a consumer changelog "for now, not forever" with the trigger named: *revisit if/when a version is actually cut*. Both of its grounds have expired (ADR 0019 gives a version scope; there is a release), so the file exists — ADR 0020, and 0016 has a pointer to it | now |
| 26 | pkg.go.dev first impression (`t11-pkg-go-dev-first-impression.md`) | Not previously on any list, and invisible from inside the repo: pkg.go.dev shows a subdirectory module's **package doc**, not `README.md`, so `core`'s entire landing page is `core/bandwidth.go:1-4` — which still says "local, Redis, Postgres" — plus an empty Examples section (`grep -rn '^func Example' .` returns nothing). Fix the comment, add the bucket4j-lineage sentence ADR 0019 routes here, add one runnable `Example`. `CONTRIBUTING`/`SECURITY.md` ruled **out** of v0.1.0 in the same ticket: private, single-author repo, so their whole audience appears only when blocker (a) flips | now |

**Rows marked `_(done)_` keep the `path:line` that was their evidence when the
defect was found.** Those are a record of what was wrong, not live pointers, and
they are not re-verified — the shipped fix moved most of them. Only rows without
that marker carry citations you can act on.

Row 21 is AC-53 (negative `AvailableTokens`). Its spec was rescued off an
unmerged branch and shipped with T03; the spec is now in
`ai-context/done/core-available-tokens-negative.md`, and the
row itself is folded into row 22's ticket, `t03-reservation-semantics.md`,
since both are one change to the same mechanism and both rest on ADR 0018. Ids
are stable, so the gap in the table is left rather than renumbered.

Row 14 is stated more narrowly than the audit that raised it: the 429 body is
already a fixed string, so the leak is on the 503 path, not the 429 one.

The release rows (11, 15, 18, 24, 25, 26) are all `now`, but they are not
parallel: 24, 25, 26, 11 and 18 land in any order, **then** 15 cuts the tags,
because a tag cannot be moved once anyone has fetched it. The step sequence is
in `publish-v0.1.0.md`, not duplicated here. `AC-61` is taken by row 15 and is
the only new AC id in the release round — the other five rows produce
artefacts (a file, a comment, a paragraph), not behavioural criteria, and
minting ids for those would dilute a registry whose only index is
`grep -rn 'AC-'`.

## Later / unscheduled

Where this library sits: the Go ecosystem's limiters are overwhelmingly either
local-only (`golang.org/x/time/rate`, `uber-go/ratelimit`) or single-backend
(`go-redis/redis_rate` is Redis-only). A pluggable multi-store design with one
conformance suite proving the swap is genuinely unusual, and that is the thing
worth protecting when scope pressure arrives.

Three gaps against what consumers now expect, none of them v1:

- **OpenTelemetry metrics and tracing hooks.** Almost no Go limiter offers
  them, which makes it a differentiator rather than table stakes. Constrained
  by ADR 0002: `core` takes no transport dependency, and an OTel dependency in
  `core` would break the zero-runtime-deps property — so any design starts from
  a callback or a separate module, not an import.
- **IETF `RateLimit-*` response headers** (`draft-ietf-httpapi-ratelimit-headers`,
  an active Internet-Draft already deployed de facto). Belongs in the caller's
  adapter, same reasoning as ADR 0002; what `core` owes is the numbers, which
  `ConsumptionProbe` mostly already carries.
- **Multi-bandwidth** — deferred out of v1 by ADR 0001, and now **planned for
  v0.2** by owner ruling (2). That does not contradict the ADR: it deferred the
  surface out of v1, it did not rule it out. It is row 17, together with the
  config fingerprint, because both need the same format break.

One claim from that research — **"Redis 8.8+ has native GCRA"** — is unverified
and looks wrong. It must be checked against Redis release notes before it is
cited anywhere, and it must not reach an ADR unchecked.
