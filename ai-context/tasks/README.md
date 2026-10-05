# tasks/

One file per unit of work, created by the Architect for Dev or the
Investigator. A task moves to `../done/` only when the **tester** agent
(`.claude/agents/tester.md`) has independently confirmed both `/verify` and a
real end-to-end run pass — never moved by whoever did the work. See
`docs/adr/0007-tasks-done-workflow.md` for why this replaced `docs/internal/`.

## Required sections

A task file states the problem, what is in scope, what "done" looks like, and
— since `docs/adr/0013-architect-scopes-test-coverage.md` — a **Test
scenarios** section. That section is the Architect's, and it names the
coverage both other roles work from:

- **Black-box (tester, Gate 2):** the scenarios to exercise through
  `e2e/k6/run-sandbox.sh <backend>` — request sequence and the observable
  outcome (status codes, accept/reject counts) that must hold. The tester runs
  these and does not invent others.
- **Unit / integration (Dev):** the cases Dev must pin with tests while
  implementing, named by behavior and by AC id where one exists.

## Citations go stale, and a stale one costs more than no citation

Task files are mostly `path:line` evidence, and every merged commit ages some of
it. Two rules, both learned from getting it wrong:

- **Re-verify every citation in a file you edit, not just the lines you touch.**
  Editing by targeted string replacement is how a citation gets flagged as stale
  in one file and left stale in another during the same commit. If you open a
  file, you own all of its references for that commit.
- **Cite the symbol when the line number adds nothing.** `casBackoff`
  (`core/command.go`) survives a refactor; `core/command.go:59` does not, and a
  wrong line number is worse than no line number — it sends a reader to real
  code that is not the code being discussed, which reads as correct. Reserve
  bare `path:line` for a specific line that is itself the point (an unclamped
  return, a missing guard) and pair it with the symbol name even then.

A file that says its citations were re-checked, without them having been, is the
failure this is written about.

Describe behavior, not test mechanics — Dev owns how the tests are written.
If a task has no black-box surface (pure doc or naming change, or a change
confined to `core`'s local path), say that in one line under the black-box
bullet instead of dropping the section.
