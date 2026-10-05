# T07 — Apache-2.0 `LICENSE` in seven roots, one `NOTICE`

**For:** Dev. Mechanical, no Go code. **Blocks** `publish-v0.1.0.md` — a tag
without a recognisable license is a tag that has to be superseded.

**Ruled in** `docs/adr/0019-v0-1-0-is-five-lockstep-modules-under-apache-2-0.md`.
The license choice and the NOTICE question are settled there; do not re-open
them in the PR. (0019's lockstep section is superseded by
`docs/adr/0021-modules-version-independently-after-v0-1-0.md` — the license and
NOTICE rulings this ticket implements are untouched by that.)

## Why seven copies

A Go module is fetched on its own. The only `LICENSE` a consumer of
`redisstore` ever receives is the one inside `redisstore/`, and pkg.go.dev
renders no documentation at all for a module whose license it cannot
recognise. `gh repo view --json licenseInfo` currently returns `null`.

## In scope

- Verbatim Apache-2.0 text as `LICENSE` in: repo root, `core/`, `redisstore/`,
  `pgstore/`, `mongostore/`, `etcdstore/`, `examples/`. Seven identical files.
  Take the text from apache.org, not from a package manager's copy — the
  "APPENDIX: How to apply" section stays, that is part of the canonical text.
- Fill the copyright line in the appendix boilerplate *only if* using the
  per-file header form. **Do not add per-file license headers** to the Go
  sources: Go projects overwhelmingly do not, and 40-odd files gain a
  twelve-line banner for nothing.
- One `NOTICE`, repo root only, three or four lines: this library carries
  bucket4j's semantics, no bucket4j source was copied, bucket4j is
  Apache-2.0 and copyright its authors. Point at
  `docs/adr/0018-deliberate-deviations-from-bucket4j.md` for the detail.
- The copyright holder line in `NOTICE` — the repo owner's name/handle and the
  year. If the right string is not obvious, ask rather than guess.

## Out of scope

- Any `NOTICE` in a module root (ADR 0019: bucket4j ships no `NOTICE`, so
  §4(d) propagates nothing; one voluntary attribution file is enough).
- The lineage sentence in `core`'s package doc — that is `t11`, which is
  already editing that comment.
- `README.md`'s license section. If one is wanted it belongs with `t08`'s
  README pass, not here; a `LICENSE` file is self-describing to both GitHub
  and pkg.go.dev without it.
- `.github/` anything.

**Done looks like:** seven `LICENSE` files with byte-identical content, one
root `NOTICE`, `gh repo view --json licenseInfo` reporting Apache-2.0, and
`/verify` still green (it should be untouched — if it is not, something was
put inside a package directory that Go tried to build).

## Test scenarios

**Black-box (tester, Gate 2):** no black-box surface — no Go code, no
behaviour change. Run `e2e/k6/run-sandbox.sh redis` once as a no-regression
check (5 accepted, 3 rejected; refill checks pass). The substantive check is
mechanical and belongs in the review: seven files, identical bytes
(`sha256sum */LICENSE LICENSE` gives one hash), one `NOTICE`.

**Unit / integration (Dev):** none. A test asserting a file exists would pin
nothing a reviewer cannot see, and no AC id is taken for the same reason —
the AC registry is for behavioural criteria referenced from test names, not
for artefacts.
