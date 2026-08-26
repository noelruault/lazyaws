# Built ledger — TuiRedesign

One line per shipped ticket, appended by the builder: `- <id> <sha> — summary`.

- s1-cell-rendertablefit a0bcb37 — utils.Cell + RenderTableFit: weighted, width-budgeted table with escape-safe truncation
- s1-primitives 8c9f314 — presentation Badge/Gauge/RelTime/SectionTitle/ResourceHeader, formatByteCount moved and aliased
- s1-columns 50e891f — presentation.Columns zips two blocks with an escape-preserving cut, stacks below minTwoColWidth
- s2-inpanel-empty 21c5126 — RerenderList writes the muted NoItemsMessage into the side view; messages shortened
- s2-left-profiles-ecs a1c3d7f — SideListPanel gains GetTableCellsFit/Weights; profiles and ECS rows on RenderTableFit, ECS clusters gain a health badge
- s2-left-ec2-s3 d30c315 — EC2 (bold name, muted id) and S3 rows on RenderTableFit; instance id flexes so a narrow panel cannot delete the name
