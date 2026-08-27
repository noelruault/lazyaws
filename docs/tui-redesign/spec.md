# Loop spec — TuiRedesign

**Branch:** `tui-redesign`. One writer.
**This file is the operating contract. The loop reads it every cycle.**

## What we are building

A single-screen redesign of the lazyaws TUI, incremental and never breaking the running app:

- Left side stays 8 stacked resource panels, but every row becomes glanceable: prominent name, muted identifiers, aligned columns, semantic status badges (icon + word, never color alone), in-panel empty states.
- Right side gets an `Overview` tab per resource, first in the tab list, consolidating what today is spread across tabs that hold one or two lines each. Existing tabs, keys, and data flows are preserved; nothing is removed.
- Overviews use a two-column text layout that collapses to one column on narrow terminals, never soft-wraps (wrap off, width-aware rendering), and every section degrades to an explicit `unavailable` state instead of inventing data.
- ECS views must show the container image a deployment is actually running (from DescribeTasks container data; desired image from the task definition when nothing runs, labeled as such).
- Tiered auto-refresh: focused panel list and open overview every ~2s, CloudWatch metrics every ~60s (GetMetricData is billed per metric requested), single-flight per panel, adaptive SDK retry, backoff on throttle errors. Manual `r`/`R` refresh keeps working.
- Selection is preserved by resource identity across reloads, never by index; the detail pane must never describe a different resource than the visibly selected row.

The reference TUI quality bar is k9s/lazygit information hierarchy. It must still look and behave like a TUI: no gradients, no boxes-in-boxes, color used sparingly and semantically.

## Green gate (trust exit codes, from repo root)

A cycle may only commit if ALL of these exit 0:

```
make lint
make test
```

Noisy warnings are not failures; trust `$?`. **Never commit red.** Non-trivial pure logic leaves ONE assert-based unit test wired into the gate.

Connected verification (best-effort, never part of the gate): when the locally configured read-only staging AWS profile has live credentials, run the TUI against it and eyeball the ticket's surface; record what was checked in the handoff. If credentials are expired, say so in the handoff and move on; never block or fail a cycle on missing credentials, and never perform AWS write operations to test UI behavior.

## Definition of Done (the builder only stops when ALL hold)

The terminal `final-dod` ticket emits the literal phrase `backlog empty` ONLY when:

- every backlog ticket is in built.md;
- every group has been reviewed in-cycle and carries a `- reviewed <id> <sha>: …` line in `review.md`;
- the full green gate passes end-to-end;
- benchmarks exist and run for the fit-table renderer, the column zipper, each overview formatter, and the list rerender path;
- the README key table is regenerated if any key binding changed (`TestReadmeKeyTableIsCurrent` green);
- the existing arrangement/layout invariant tests pass unmodified;
- every overview formatter has tests covering empty, error/partial, and missing-optional-field states;
- the Playwright journey suite (`make ui-test`) passes end-to-end against the seeded fake-AWS endpoint. It is part of the DoD and of the stage-10 tickets' own verification, NOT of the per-cycle `make lint test` gate: it needs ttyd, Node and a local emulator, and a cycle on a machine without them must not go red for that.

If any item is not yet true, KEEP LOOPING — split the gap into new append-only tickets.

## Out of scope (never becomes a ticket)

- Any AWS mutation (start/stop/delete/rotate/put) for testing purposes; this loop is UI + read paths only.
- Pushing to or rebasing `main`; this loop owns only `tui-redesign`.
- Replacing gocui or any TUI framework change.
- New third-party dependencies, including clipboard libraries (copy uses the existing popup pattern).
- Editing `.plans/` or referencing local planning documents from code, comments, commits, or committed docs.
- Reworking the EKS N+1 fetch pattern or S3 pagination gaps (pre-existing, tracked elsewhere).

## Pipeline conventions (baked in — do not re-derive)

- **One loop, one branch (`tui-redesign`), one group per cycle, green-only.**
- **Review is a BLOCKING step inside the cycle**, not a second loop: the same cycle that builds a group audits it against the handoff and the diff, FIXES what it finds, records one line in `review.md`, and only then closes the group. It never files a review ticket — a review queue costs a cycle of orientation per finding and grows without bound.
- ids are **append-only + stable**. Never renumber/delete.
- Comments explain why, never what; markdown and comments are never hard-wrapped to a column; commit subjects are plain descriptive English (no `type(scope):` prefixes — a local hook blocks them).

## Running the loop

Everything the loop needs is in this directory: this spec, `backlog.md`, the ledgers, the cycle prompt and runner config in `loop/`, and the selector scripts in `scripts/` at the repo root.

With loopctl (preferred; handles accounts, usage-limit waits, stall guards, cost ledgering): symlink or copy `loop/tui-redesign.{loop,prompt}` into your loopctl checkout's `loops/`, set `TARGET`/`LOG` in the `.loop` to your paths, then `loopctl tui-redesign start`. The files here are the source of truth; loopctl only discovers them.

Without loopctl, a cycle is just one `claude -p` run of the prompt from the repo root on the `tui-redesign` branch:

```
while :; do
  out="$(claude -p "$(cat docs/tui-redesign/loop/tui-redesign.prompt)")"
  printf '%s\n' "$out"
  case "$out" in *"backlog empty"*) break;; *"pause:"*) break;; esac
done
```

Requirements: the `claude` CLI logged in, Go toolchain for the gate (`make lint test`), and optionally the `router` CLI for in-cycle delegation (cycles skip delegation cleanly when it is absent). AWS credentials are only needed for the best-effort connected checks; cycles proceed without them.
