# Built ledger — TuiRedesign

One line per shipped ticket, appended by the builder: `- <id> <sha> — summary`.

- s1-cell-rendertablefit a0bcb37 — utils.Cell + RenderTableFit: weighted, width-budgeted table with escape-safe truncation
- s1-primitives 8c9f314 — presentation Badge/Gauge/RelTime/SectionTitle/ResourceHeader, formatByteCount moved and aliased
- s1-columns 50e891f — presentation.Columns zips two blocks with an escape-preserving cut, stacks below minTwoColWidth
