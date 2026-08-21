# Backlog — TuiRedesign (tui-redesign)

Build the topmost unbuilt `- [ ]` id not already in `built.md`. ids are **append-only + stable**
(never renumber/delete). Priority order top to bottom. Each must pass the green gate (spec.md).
Every group is reviewed and repaired inside the cycle that built it (spec.md), so nothing else queues.

<!-- Replace the example with real tickets. Keep the final-dod ticket LAST. -->

- [ ] `t-01-example` — <what to build> + <acceptance: how to know it's done> + <the test it leaves>.

## Terminal

- [ ] `final-dod` — HUGE. **The only ticket that may emit "backlog empty", and it is dispatched ALONE** (that is what the HUGE token buys: batched with other tickets, a cycle could reach the stop sentinel while its group was still open). Confirm every group carries a
  `- reviewed <id>` line in `review.md`, then that the full Definition of Done (spec.md) holds and the
  green gate passes end-to-end. If
  ANY item fails, file append-only fix tickets and KEEP LOOPING. Only when every item passes, end the
  cycle with the literal phrase `backlog empty`.

<!-- Tickets are dispatched in GROUPS (default 3 per cycle), because a
     cycle's cost is orientation, not the edit. A ticket that genuinely fills a whole cycle on its own
     gets the token HUGE somewhere on its line and is then dispatched alone. Use it sparingly. -->

<!-- DIFFICULTY: give every ticket a `[dN]` marker, N in 1..5. The runner routes the cycle's model
     from the top unbuilt ticket's marker (d1-2 cheap, d3-4 mid, d5 strongest), which is measured at
     -70% cost. An unannotated backlog silently runs everything on the default model.
     For a GROUP, mark it with its HARDEST member's difficulty — never under-power a group — and try
     to draw groups so their members sit in one band, or the cheap members subsidise nothing. -->
