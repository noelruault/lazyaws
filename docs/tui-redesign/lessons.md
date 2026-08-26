# Lessons — TuiRedesign

Durable lessons only: a gate invocation that works here, a repo gotcha, a class of ticket that keeps failing and why. Read every cycle, so keep it tight.

- Gate is `make lint` then `make test` from the repo root, both exit 0 on a clean tree; `make bench` covers only `./ui/ ./ui/resources/ ./ui/fuzzy/`, so a benchmark added under `ui/utils` or `ui/presentation` will not run until that target's package list grows.
- For width/truncation code, an exact-string test pins one width and proves little about the arithmetic. Pair it with a sweep over every width (plus wide runes and colour) asserting the visible width never exceeds the budget. It found no live bug in stage 1, but mutation-testing showed it kills the whole off-by-one class the per-case expectations sail past: a dropped budget clamp, a forgotten column separator, a forgotten rule cell, a forgotten ellipsis cell.
- Audit a diff by mutation, not by re-reading it. Break one invariant, run the one test that should fail, revert. A surviving mutant is a test that asserts nothing, and it is the cheapest way to tell a real assertion from a plausible-looking one.
- `runewidth.Truncate(s, w, tail)` subtracts the tail's width from `w` and returns just the tail when nothing fits, so at `w == 0` it emits a one-cell `…` and overruns the column. Guard `width <= 0` before calling it.
- Verify a test's own arithmetic against the code, not the reverse: `\x1b[1;32m` is 7 bytes, and a hand-computed 8 sent the first red gate. When a single new assertion fails, suspect the expectation before the implementation.
- Colour and truncation do not commute. Truncate plain text and colour the result; cutting an already-coloured string splits an escape pair and bleeds into the next column. Where a pre-rendered line must be cut (`presentation.truncateStyled`), copy CSI sequences through untouched, count only visible cells, and close the cut with a reset.
