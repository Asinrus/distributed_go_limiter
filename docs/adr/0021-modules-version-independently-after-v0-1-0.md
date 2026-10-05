# Modules version independently after v0.1.0; `core`'s interfaces are the contract

`docs/adr/0019-v0-1-0-is-five-lockstep-modules-under-apache-2-0.md` ruled that
the five library modules "move together from here". The owner has reversed
that half of 0019, after asking the question it turns on — *"can we release
`core` independent from vendors?"* — and being shown the coupling measured
rather than assumed.

All five still **start** at `v0.1.0`; the first release is unchanged. From
that tag on, each module is tagged on its own, when its own code changes:
Go's default. `examples` is still never tagged, for 0019's reason.

0019's other three rulings — Apache-2.0 in seven roots, one root `NOTICE` plus
a lineage sentence in `core`'s package doc, and tagged-but-unpublished — are
untouched.

## What changed the answer: the coupling is seven symbols, all of them stable

Every one of the four stores uses exactly the same seven `core` symbols in its
non-test code, and nothing else:

```
CASStore / LockStore          NewCASProxyManager / NewLockProxyManager
ProxyManager                  ProxyOption
WithClock  WithExpireAfterWrite  WithMaxCASRetries
```

(`redisstore/redisstore.go`, `pgstore/pgstore.go`, `mongostore/mongostore.go`,
`etcdstore/etcdstore.go` — two interfaces, two constructors, one struct, one
option type and three options.) That is the whole surface, and it is the
surface the repo exists to keep stable: it is what "swap the provider without
changing any rate-limiting logic" *means*.

The decisive case is the break that is actually coming. Multi-bandwidth
(`docs/adr/0001-single-bandwidth-v1.md`) changes the persisted byte layout in
`core/codec.go` before v1.0. It touches **none** of the seven, because a store
is a byte-blob KV that never parses state — the token math never runs in the
store. So the largest planned `core` change needs no store release at all, and
lockstep would spend four no-op re-tags on it. 0019 called that cost
acceptable because it bought a tested combination. It did not.

## Lockstep's guarantee was not enforceable, and this is the honest version

0019: *"the same number everywhere means the combination was tested together."*
That holds only while a consumer pins all five at the same number, and nothing
makes them. Go resolves a dependency to the **highest** version any module in
the build requires, so a consumer who imports `redisstore v0.1.0` alongside
`pgstore v0.2.0` gets `core v0.2.0` underneath a `redisstore` that was built
and tested against `core v0.1.0` — a combination lockstep never tested and
could not refuse. Lockstep was a naming convention that *looked* like a
guarantee. Dropping it loses a convention, not a property.

So state the guarantee as what it is:

- **The contract is `core`'s exported interfaces**, not a version number.
  `CASStore` and `LockStore` are what a store implements;
  `NewCASProxyManager` / `NewLockProxyManager` / `ProxyManager` /
  `ProxyOption` and the three options are what it calls. A consumer can rely
  on those being stable across any `core` release that does not say otherwise
  in `CHANGELOG.md`, independently of what else moves in `core`.
- **A store's own `go.mod` names the `core` it was tested against.** That one
  pair is the pair with a test result behind it — `redisstore/go.mod`'s
  `require` line is the answer to "which `core`?", in one file, not five.
- **Any pair is checkable, not guessable**: `cd <store> && GOWORK=off go test
  ./...` resolves whatever `core` the build selects and runs the shared
  conformance suite (`core/storetest`) against a real container of that
  store's vendor. The suite is the executable form of the acceptance criteria,
  it ships inside `core`, and it exercises more of `core` than the seven
  symbols do (`NewConfig`, `NewBandwidth`, `ErrInvalidConfig`, `storetest.Run`
  — the store *test* blast radius, wider than the consumer one). A pair that
  passes it is supported. That is a stronger statement than lockstep's, and
  unlike lockstep's it is a command.
- **When `core` breaks compatibility**, the seven symbols above are the blast
  radius to check, and a `CHANGELOG.md` entry naming which of them moved is
  the release note that matters. If none moved, no store needs a release and
  none gets one. If one did, every store needs a new tag, and the pre-v1.0
  freedom to do that is why the break should happen before v1.0.

One hazard that neither scheme ever addressed, said plainly because it gets
confused with this one: two nodes running **different `core` versions against
the same key** will disagree the moment the byte layout changes. That is a
rollout problem between `core` and `core`, not between `core` and a store;
lockstep did not help it and independent versioning does not worsen it.
`CHANGELOG.md`'s known-limitation line on the stored layout is where a
consumer is warned.

## Consequences for the changelog

One `CHANGELOG.md` stays one file
(`docs/adr/0020-changelog-starts-at-v0-1-0.md`), but its premise shifts: it is
one file because this is one repo with one shared contract, not because there
is one number. Entries say which module and which version a change lands in.
0020's rejection of five per-module changelogs stands on the thinner ground —
five files to maintain for four modules that change rarely and only ever
against the same seven symbols.

Rejected: **keeping lockstep.** It is cheaper to explain in one sentence, and
that sentence is the problem: it promises a tested combination it cannot
enforce, which is worse than no promise. The real cost of dropping it is that
`v0.3.1` of a store no longer tells you anything about `core` at a glance —
answered above by pointing at the store's `go.mod` and a runnable check.

Rejected: **a compatibility matrix document.** This is 0019's objection to
independent versioning, and it was right that nobody maintains a matrix by
hand. The answer is not to collapse the matrix with a convention but to make
it unnecessary: one axis (`core`), one authoritative line per store
(`go.mod`), one command to verify. A table in a markdown file would go stale
the first time a tag was cut without editing it.

Rejected: **a `core` major bump for every layout change**, so that pairs could
never mix. Pre-v1.0 that is what `v0.x` already allows without the ceremony,
and post-v1.0 it would mean `/v2` import paths in all four stores for a change
that does not touch their code.

Rejected: **renaming 0019's file** to stop it asserting a policy it no longer
holds. `docs/adr/0018-deliberate-deviations-from-bucket4j.md` already ruled
this trade-off for its own misnamed slug: the number is the stable identifier,
the slug is decoration, and renaming breaks inbound links that cost more than
the wrong word. Five committed files link 0019 by full path
(`docs/adr/0016`, `docs/adr/0020`, `ai-context/tasks/t07-apache-2-0-license.md`,
`ai-context/tasks/publish-v0.1.0.md`, `ai-context/tasks/next-roadmap.md`),
two unmerged branches add more, and the merged PR title is already in
`git log`. 0019's filename reads as the state of the world on the day it was
ratified, which is what an ADR filename is. The supersession pointer inside it
is what a reader follows.
