# T11 — What a stranger sees first: `core`'s package doc and one runnable example

**For:** Dev. Go comments plus one new `_test.go` file. **Blocks**
`publish-v0.1.0.md`.

Not on the original release list, added because it is the only release gap
that is invisible from inside the repo. pkg.go.dev does not show `README.md`
for a module in a subdirectory — it shows the **package doc comment** and the
**examples**. For `github.com/asinrus/distributed_go_limiter/core`, the entire
landing page is therefore `core/bandwidth.go:1-4` plus an empty Examples
section. Both are wrong today: the comment names three backends out of four
("storage providers (local, Redis, Postgres)") and there is not a single
`func Example` anywhere in the repo (`grep -rn '^func Example' .` returns
nothing).

Every backend package already has a decent doc comment
(`redisstore/redisstore.go:1-4` and siblings). `core` — the one module every
consumer takes — is the one that is stale.

## In scope

- `core/bandwidth.go:1-4`, the `core` package doc. Fix the backend list, and
  do it without naming the backends one by one if possible — an enumeration in
  `core`'s own doc comment is what went stale, and `core` is deliberately
  vendor-agnostic (roadmap row 19), so "a storage backend supplies an atomic
  store; this repo ships four" rots more slowly than four names.
- One sentence in the same comment on the lineage: semantics rewritten from
  bucket4j, no bucket4j source copied, Apache-2.0 both sides. This is the
  module-fetch audience's copy of the root `NOTICE` (`t07`), and ADR 0019
  routes it here deliberately — it is the one place a consumer who never sees
  the repo will read it. Keep it to one sentence; the detail is ADR 0018's.
- One runnable `Example` in `core`, in a new `core/example_test.go`,
  **`package core_test`** — the external test package, so the example can only
  reach the exported surface and therefore cannot show a reader code they
  cannot write. Construct a `Config`, a bucket, consume until rejected, with
  an `// Output:` block. Use the in-memory path (`core.NewLocalBucket` or
  `core.NewInMemoryProxyManager`) so it needs no container and runs in
  `/verify`.
- If one example is cheap, a second on `NewInMemoryProxyManager` is worth it —
  it is the shortest honest demonstration of the provider-swap claim, since
  the only line that would differ for Redis is the constructor. Dev's call
  whether that reads better as a second example or a comment in the first.

## Out of scope

- `CONTRIBUTING.md` and `SECURITY.md`. **Ruled out of v0.1.0:** the repo is
  private and single-author, so there is no contributor to onboard, and
  GitHub's private vulnerability reporting needs a public repo — both are
  work whose entire audience appears at the same moment blocker (a) is
  flipped. Revisit then, not now.
- A root `go.mod`. The root stays `go.work`-only (ADR 0019's five-module
  shape). The consequence to accept rather than fix: pkg.go.dev will have no
  page for `github.com/asinrus/distributed_go_limiter` itself, only the five
  module pages. That is normal for a multi-module repo and inventing a sixth
  module to hold a landing page is not worth a version to tag.
- Per-backend examples. They need a live vendor, so they cannot be runnable
  `Example` functions, and `examples/` plus `docs/client/*` already cover
  that ground.
- Doc comments on backend packages — already fine.
- `README.md` — `t08`.

**Done looks like:** `go doc github.com/asinrus/distributed_go_limiter/core`
reads as an accurate introduction to the library with no wrong backend count,
and `cd core && go test -run Example ./...` runs at least one example and
checks its output.

## Test scenarios

**Black-box (tester, Gate 2):** no black-box surface — a doc comment and an
example test, no production behaviour change. Run
`e2e/k6/run-sandbox.sh redis` once as a no-regression pass (5 accepted, 3
rejected; refill checks pass).

**Unit / integration (Dev):** the example *is* the test — an `Example` with an
`// Output:` block is compiled and its stdout compared, so the snippet cannot
rot the way `README.md:8-23` silently has. Cases it must cover, because they
are the two things a reader needs to see work:

- a bucket with a known capacity accepts exactly that many immediate
  `TryConsume(1)` calls and rejects the next one;
- after the refill interval, it accepts again. **`WithClock` is not reachable
  for `NewLocalBucket`** — that constructor takes no options
  (`core/local.go:26`) and clock injection there is the unexported
  `newLocalBucketClock` (`core/local.go:34`), which `package core_test` cannot
  call. So either write the refill half against
  `NewInMemoryProxyManager(core.WithClock(...))`, which *is* exported
  (`core/proxymanager.go:31`), or leave refill to the existing `core` tests
  and keep the example at the reject boundary. Do not add a real `time.Sleep`
  to an example — a slow example is one nobody runs, and do not export a
  local-bucket clock option just to make an example work.

No AC id. The behaviour here is already AC-11..AC-19 territory; this ticket
documents it, it does not add a criterion.
