# tasks/

One file per unit of work, created by the Architect for Dev or the
Investigator. A task moves to `../done/` only when the **tester** agent
(`.claude/agents/tester.md`) has independently confirmed both `/verify` and a
real end-to-end run pass — never moved by whoever did the work. See
`docs/adr/0007-tasks-done-workflow.md` for why this replaced `docs/internal/`.
