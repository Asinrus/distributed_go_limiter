# T09 — State the stored format may change before v1.0 (row 18, release blocker)

**For:** Dev. Prose, plus one comment line in a test. **Blocks**
`publish-v0.1.0.md`.

**This is roadmap row 18**, promoted out of tier 3. The row already carried the
sequencing argument and it is the whole reason this is a blocker: *a format
break is only safe if the promise was made first.* Multi-bandwidth is planned
for v0.2 (row 17, ADR 0001) and will change `core/codec.go`'s byte layout. If
v0.1.0 ships silent, the v0.2 break is a breaking surprise to anyone who
upgraded; if v0.1.0 says the layout is not part of the API, the same break is
a documented consequence. The cost of saying it is one paragraph; the cost of
not saying it is the only kind of reputational damage a 0.x library can
actually do.

## What to say

The substance, in the project's own voice — not a legal disclaimer:

- What is stored is a private wire format between this library and itself, not
  a public interface. Today: one version byte plus three big-endian int64s, 25
  bytes (`core/codec.go:8-15`).
- It **will** change before v1.0 — multi-bandwidth needs N states where there
  is now one.
- The version byte is the mechanism: `decodeState` rejects an unknown version
  rather than misreading it, so a mixed-version fleet fails loudly on the
  affected keys instead of enforcing a wrong limit silently.
- What a consumer should therefore not do: parse the bytes, migrate them by
  hand, or expect a rolling upgrade across a format change to preserve
  existing buckets. Draining or letting the keys expire is the supported path.
- Pre-v1.0 generally: the `v0.x` tag means the Go compatibility promise does
  not bind yet. One sentence, consistent with `t08`'s Status line — agree the
  wording once between the two tickets rather than phrasing it twice.

## Where it goes

The row says `docs/client/*` "each have a 'what is stored' section". **That is
only half true and the difference matters:** only `docs/client/redis.md:54` and
`docs/client/etcd.md:41` have `## What is stored`. `docs/client/postgres.md:28`
and `docs/client/mongodb.md:22` have `## Schema` instead — same role, different
name, because a row and a document are describable where an opaque blob is not.

So: add the promise to all four, under whichever heading that file already
uses, and do not rename the headings to match — `## Schema` is the right word
for a table definition a user has to apply themselves
(`docs/client/postgres.md:46` "Applying it").

Keep it short in each file and do not copy five paragraphs four times. One
short paragraph per file, phrased the same, is acceptable duplication for
documentation a reader arrives at from four different entry points.

## Out of scope

- Any change to `core/codec.go`, the format itself, or the version byte.
- Designing the v0.2 layout. Row 17, its own ticket, probably its own ADR.
- A codec stability *guarantee* for v1.0. That is a promise to make when v1.0
  is in sight, not now.
- An ADR. The decision being recorded is "we will say the thing the roadmap
  already decided we would say" — it is not hard to reverse, not surprising,
  and carries no trade-off. ADR 0007's bar is not met.

**Done looks like:** all four `docs/client/*.md` state that the stored layout
may change before v1.0 and that it must not be parsed, and
`core/codec_test.go:44`'s `TestCodecWireFormatIsFrozen` has a comment line
pointing at the client docs, so that whoever trips that test at v0.2 is told
where the promise they are about to cash in lives.

## Test scenarios

**Black-box (tester, Gate 2):** no black-box surface — documentation only, no
behaviour change. Run `e2e/k6/run-sandbox.sh redis` once as a no-regression
pass (5 accepted, 3 rejected; refill checks pass).

**Unit / integration (Dev):** no new tests, and no AC id. The claim the docs
make is **already** pinned by `core/codec_test.go:44`
(`TestCodecWireFormatIsFrozen` asserts the exact 25 bytes and the version
byte) and `core/codec_test.go:17` (`require.Len(t, data, stateByteSize)`). That
is why this ticket adds prose and not a test: the tripwire exists, it just
does not tell the person who trips it what to do. The one check Dev owes is
that the byte count and the version number written into the docs match
`core/codec.go:13-14` — a doc that states the wrong size is worse than one
that states nothing.
