# The Architect scopes a task's test coverage, in the task file

Until now a task file said what to build and what "done" looks like, but not
what to *test*. Dev derived unit/integration cases from the code it was
writing; the tester derived an end-to-end scenario from the task prose — and
`.claude/agents/tester.md` simultaneously forbids it from inventing scenarios.
Two roles guessing the same coverage independently is how a case ends up
covered twice at the unit level and zero times through the compiled binary,
which is exactly the failure mode ADR 0011 was written about.

So the Architect now names the intended coverage when it scopes the task: a
**Test scenarios** section, written from the outside-in (observable behavior:
inputs, expected accept/reject, expected error), listing the black-box cases
the tester must see through the k6 sandbox and the cases Dev must pin with
unit or integration tests. The Architect does not write the tests or dictate
their mechanics — it states the behavior that must be covered and which gate
should cover it. Dev still owns test code (`.claude/agents/dev.md`); the
tester still owns the gate.

Rejected: **letting the tester author its own scenarios.** It has no `Read`
tools by design (ADR 0011), so it would be inventing behavior from prose —
and a scenario invented by the same run that grades it isn't independent.

Rejected: **a separate spec document per task.** The acceptance criteria are
already the contract, the task file is already the unit of work, and a third
artifact is one more thing to drift. One section in a file that already has to
be written costs nothing.

A task with no meaningful black-box surface (pure doc, naming, or a change
confined to `core`'s local path) says so in that section — one line — rather
than omitting it. An absent section is a task that wasn't finished being
scoped; an explicit "no black-box surface" is a decision the tester can act on.
