# T02 — Rename the conformance suite: `core/tck` → `core/storetest`

**Blocked by:** T01 — both edit `CLAUDE.md`, and T01 deliberately leaves the
`core/tck` references there untouched so this ticket moves them in one place.

**For:** Dev. Pure rename, no behaviour change.

## Why

"TCK" is Java jargon — a Technology Compatibility Kit is a JCP artifact, and
the name arrived with the bucket4j port rather than with any Go convention. Go
names this kind of package `<thing>test`: `net/http/httptest`,
`testing/fstest`, `io/fs`'s `fstest.TestFS`. A Go developer reading
`core/storetest` knows what it is without being told; `core/tck` has to be
explained every time, and `ai-context/README.md`, `CLAUDE.md` and `README.md`
all currently explain it.

**It stays a normal (non-`_test.go`) package inside the `core` module.** Two
constraints, both real:

- Go cannot share `_test.go` code across module boundaries. `redisstore`,
  `pgstore`, `mongostore` and `etcdstore` are separate modules; the only way
  they can call the same suite is if it is an importable package.
- It uses only `core`'s **exported** API. That is not a limitation to apologise
  for — what the suite proves is a claim about the public surface (a caller can
  swap the store without changing rate-limiting logic, AC-46), so testing it
  through anything but the public surface would prove the wrong thing.

That combination means a non-test package importing `testing`, which
`CLAUDE.md`'s own conventions flag. Say why in the package doc — it is the
justification above, in two sentences — so the next reader does not file it as
a defect.

## In scope

- `core/tck/tck.go` → `core/storetest/storetest.go`, `package tck` →
  `package storetest`. The exported entry point stays `Run`, so call sites
  change only in the package qualifier.
- The four backend call sites: `redisstore/redisstore_test.go:40`,
  `pgstore/pgstore_test.go:50`, `mongostore/mongostore_test.go:86`,
  `etcdstore/etcdstore_test.go:102`, plus the import line in each.
- **A fifth site the brief for this ticket did not list:**
  `core/proxy_tck_test.go:8,16` — the in-memory AC-46 run, inside `core`
  itself. Rename the file to `core/proxy_storetest_test.go` while you are
  there. Missing it would not compile, but it is worth knowing the count is
  five, not four.
- Prose references: `CLAUDE.md` (the `tck/` layout bullet, the "New distributed
  backend" convention bullet, the `core` test comment in the Build & test
  block), `ai-context/README.md:9-11`, `README.md`,
  `docs/adr/0006-testcontainers-for-backend-tck.md` — **do not rename the ADR
  file**; ADR filenames are stable identifiers. Add one line to its body noting
  the package's new name.
- `core/command.go:41` has a `core/tck` reference inside a comment.

## Out of scope

- Any change to what the suite asserts. That is T05.
- Renaming `Run` or any other exported identifier.
- The word "provider" inside the frozen provider-swap / AC-46 phrase — T01's
  ruling 8 keeps that wording.

## Test scenarios

**Black-box (tester, Gate 2):** none — no black-box surface; this is a package
rename with no behaviour change. Run `e2e/k6/run-sandbox.sh redis` once as a
no-regression check (5 accepted, 3 rejected; refill checks pass).

**Unit / integration (Dev):** no new tests. The check is that the *existing*
suite still runs everywhere it ran before, under its new name:
- `core`: AC-46 still passes through the in-memory manager.
- Each of `redisstore`, `pgstore`, `mongostore`, `etcdstore`: AC-41 … AC-44
  still run and pass against their real container, with the subtest names
  unchanged (`AC41_KeyIsolation` and siblings) — a rename that silently drops a
  backend's suite invocation would otherwise look like a green run.
- `grep -rn '\btck\b' .` returns nothing outside four deliberate places: the
  `docs/adr/0006-…-tck.md` filename, `ai-context/done/` (archived task files
  record what was true when the work shipped), this ticket and T01's ruling 7
  (both describe the rename, so the old name is the correct before-state), and
  `core/storetest/storetest.go`'s `tck-` test-key prefix. Anything else is a
  miss — `.claude/agents/dev.md` and `.claude/commands/verify.md` were exactly
  that, found only after this claim was first written.

**Done looks like:** `go test ./...` is green in all six modules, no reference
to `tck` survives outside the four places listed above, and the package doc says
why a non-test package imports `testing`.
