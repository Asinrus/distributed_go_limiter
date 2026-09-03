# Work items track through `ai-context/tasks/` → `ai-context/done/`, not `docs/internal/`

Design/investigation detail previously lived in `docs/internal/` — gitignored,
local-only, and consequently absent from this workspace entirely (no commit
ever added it, and no reference copy existed here). That made "the process"
undocumented anywhere a fresh clone could see it. We moved committed
decisions into `docs/adr/` and replaced the local-only problem-report
convention with one committed lifecycle: the Architect creates a task file in
`ai-context/tasks/`, Dev or the Investigator does the work, and the file only
moves to `ai-context/done/` once both `/verify` and the new **tester** agent's
end-to-end run pass — a move performed by the tester agent itself, not
self-attested by whoever did the work. `ai-context/PLANS.md` keeps the
high-level phase table and changelog; it no longer holds the decision list
(superseded by `docs/adr/`) or per-question tracking (superseded by task
files).

The `PLANS.md` clause above is superseded by
`docs/adr/0016-retire-plans-md.md`: the phase table and changelog were the
last duplicated sections, and the file is deleted rather than narrowed a third
time. Everything else here stands.
