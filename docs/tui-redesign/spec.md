# Loop spec — TuiRedesign

**Branch:** `tui-redesign`. One writer.
**This file is the operating contract. The loop reads it every cycle. FILL IT IN.**

## What we are building

<!-- Describe the goal / north star. What does "done" look like? -->

## Green gate (trust exit codes, from repo root)

A cycle may only commit if ALL of these exit 0 (edit to match this repo):

```
# e.g.
# npm run typecheck
# npm run build
# npm test
```

Noisy warnings are not failures; trust `$?`. **Never commit red.** Non-trivial pure logic leaves
ONE assert-based unit test wired into the gate.

## Definition of Done (the builder only stops when ALL hold)

The terminal `final-dod` ticket emits the literal phrase `backlog empty` ONLY when:

- every backlog ticket is in built.md;
- every group has been reviewed in-cycle and carries a `- reviewed <id> <sha>: …` line in `review.md`;
- the full green gate passes end-to-end;
- <!-- any project-specific DoD checks: coverage, perf budgets, visual/QA passes... -->

If any item is not yet true, KEEP LOOPING — split the gap into new append-only tickets.

## Out of scope (never becomes a ticket)

<!-- deploys, device tests, anything the loop must not attempt -->

## Pipeline conventions (baked in — do not re-derive)

- **One loop, one branch (`tui-redesign`), one group per cycle, green-only.**
- **Review is a BLOCKING step inside the cycle**, not a second loop: the same cycle that builds a
  group audits it against the handoff and the diff, FIXES what it finds, records one line in
  `review.md`, and only then closes the group. It never files a review ticket — a review queue costs
  a cycle of orientation per finding and grows without bound.
- ids are **append-only + stable**. Never renumber/delete.
