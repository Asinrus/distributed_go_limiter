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
task files, not to this one. Whether a queue index belongs here at all, beside
`ai-context/PLANS.md`, is the open question in
`ai-context/tasks/plans-md-vs-changelog.md`; this file does not settle it.

The `path:line` references are the evidence for each row, checked against
commit `20de7dc` (rows 5, 6 and 16 re-checked when row 6's error shape was
ratified; the rest against `5f45eb8`). If one stops resolving, the row is stale and the finding
needs re-confirming before anyone works it.

| # | Task | What it is | Tier |
|---|------|-----------|------|
| 1 | Go 1.23 pin (`go-version-1-23.md`) | One version across `go.work`, six `go.mod`, CI and docs, plus the ADR | now |
| 2 | GitHub Actions CI (`github-actions-ci.md`) | Module-discovering workflow at Go 1.23: race, vet, gofmt, govulncheck | now |
| 3 | `initialTokens` validation (`core-initial-tokens-validation.md`) | `core/bandwidth.go:50,59` — `valid()` never checks it; `n > capacity` is accepted and `core/state.go:61`'s early return means refill never clamps, so the over-capacity burst is permanent; negatives accepted too | 1 |
| 4 | etcd lease leak (`etcdstore-lease-leak.md`) | `etcdstore/etcdstore.go:39,75` — `cli.Grant` before every `Txn` including lost CAS races, leases never revoked → ~360k live leases at 100rps with the example's 1h TTL, plus 2 RPCs per token check | 1 |
| 5 | CAS retry hardening (`core-cas-retry-hardening.md`) | `core/command.go:24,45` — 100 retries with no backoff and no jitter. Kept separate from row 6 (ruling in both files): pacing has no AC and an open timer question, cancellation is a contract with two. Row 6 lands first and settles the one shared line — the backoff sleep is `ctx.Done()`-interruptible and returns the bare context error | 1 |
| 6 | Honor ctx cancellation (`core-ctx-cancellation.md`) | `core/memstore.go:43` + `core/command.go:45` — ctx discarded end to end; a cancelled ctx still consumes a token and returns success, contradicting `core/bucket.go:29`. Error shape now **decided**, not deferred: bare context error, `storeErr` (`core/command.go:17`) as the choke point (ADR 0014). Absorbs the old row-16 Q9 item — same contract, now **AC-51** alongside AC-50 | 1 |
| 7 | Widen the TCK | `core/tck/tck.go:35-38` runs only AC-41..44 — probes, estimate, `TryConsumeAsMuchAsPossible` and blocking `Consume` never run against a real backend. Tier 1 by owner ruling (3): the TCK *is* the provider-swap guarantee `README.md` advertises, and four cases do not carry it | 1 |
| 8 | `ProxyOption` on backends | `redisstore/redisstore.go:110`, `pgstore/pgstore.go:254`, `mongostore/mongostore.go:143`, `etcdstore/etcdstore.go:107` all take their own `Option` and none forwards `core.ProxyOption`, so `core.WithClock` (`core/proxymanager.go:26`) is unreachable and expiry tests burn wall-clock sleeps (`etcdstore/etcdstore_test.go:132` sleeps 3s). Tier 1 by owner ruling (3): a backend author cannot reach a core option, which is a pluggability defect, not an ergonomics one | 1 |
| 9 | _(merged into row 17)_ | Config fingerprint — `core/proxymanager.go:56-60`, config is not persisted and the codec carries none, so two nodes with mismatched config on one key silently enforce different limits. Real, but it needs a codec format change and so does multi-bandwidth; doing them separately breaks the format twice | later |
| 10 | memstore expiry sweeper | `core/memstore.go:36` — expiry is lazy-on-access only, so an untouched key is never reclaimed; unbounded memory under the per-IP keying the examples demonstrate | 2 |
| 11 | Refresh stale docs | `README.md:47-74` (three backends, no mongodb/etcd links or test commands), `CLAUDE.md:24,46-51,78-79` ("4 modules"), `ai-context/README.md:9,11,21`. Also `CLAUDE.md:58-59` — it points at acceptance criteria "in `ai-context/`" and no such file exists there (the original lived under the deleted `docs/internal/`, ADR 0007), so the AC ids have no registry beyond `grep -rn 'AC-'` | 3 |
| 12 | Fix dead paths | `ai-context/PLANS.md:150,154,179` and `ai-context/done/tester-e2e-sandbox-harness.md:13,24,45,46` point at `examples/k6/`; the harness lives at `e2e/k6/` | 3 |
| 13 | Greedy refill `ceil` | `core/state.go:158` truncates the division where it needs to round up, so a client that sleeps exactly the advertised wait is rejected again. The intervally branch two lines up (`core/state.go:138-140`) rounds up explicitly, which is the asymmetry that makes this look like a bug — but `core/state.go:6` calls the whole state a direct reduction of `BucketState64BitsInteger` (and `delayNanosForConsume`'s own doc at `core/state.go:111-113` claims nothing either way), so whoever picks this up establishes what bucket4j does *before* changing it; a deliberate deviation from the port is a different task than a fix | 3 |
| 14 | Example hardening | All four `examples/*/main.go` echo `err.Error()` into the 503 store-failure body (`examples/redis/main.go:44`), and every backend formats the bucket key into that error (`etcdstore/etcdstore.go:28` and siblings), so the client's own IP comes back to it along with the internal failure. No `Retry-After` on the 429 either, though `TryConsumeAndReturnRemaining` already has the wait | 2 |
| 15 | Publish v0.1.0 (`publish-v0.1.0.md`) | Dormant by owner ruling (1) — no near-term release. The file stays because the `replace` + `require core v0.0.0` trap is a real latent defect: it is invisible under `go.work` and would break the first outside `go get` on day one of any future release | later |
| 16 | Close open questions | The three task files still here: `plans-md-vs-changelog.md`, `vendor-hazelcast-store.md`, `q5-pgstore-identifier-behavior-change.md`. Q9 (the `ErrStore` contract AC) is gone — folded into row 6 and ratified as ADR 0014 + AC-51, because two open items on one error contract get the criterion written twice or written incomplete | later |
| 17 | Codec format change: multi-bandwidth + config fingerprint | Planned for **v0.2**. One format break carries both — the multi-bandwidth layout (N states instead of one, per ADR 0001, which deferred it from v1 and is not contradicted by planning it for v0.2) and the config fingerprint from row 9. Needs its own task file and probably an ADR before anyone writes code | later |
| 18 | Say the codec layout is unstable | Nothing states that `core/codec.go`'s byte layout may change before v1.0, yet row 17 will change it. `docs/client/redis.md` and siblings each have a "what is stored" section — the natural home. Cheap, and it must land *before* v0.2, not with it: a format break is only safe if the promise was made first | 3 |
| 19 | Cut store round-trips without vendor code | Owner constraint, ruling (3): `core` stays vendor-agnostic — no per-vendor knowledge, no backend-specific fast paths. The way to reduce calls is the command layer (`core/command.go`), so every backend inherits it; bucket4j's optimization controllers (batching, prediction, delayed writes) are the reference. **Unscoped** — `CLAUDE.md` lists optimization/batching as deferred from v1 (ADR 0001), and this needs Architect scoping and a task file before anyone implements it | later |

Row 14 is stated more narrowly than the audit that raised it: the 429 body is
already a fixed string, so the leak is on the 503 path, not the 429 one.

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
