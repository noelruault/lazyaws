# Built ledger — TuiRedesign

One line per closed ticket, appended by the builder: `- <id> <sha> — summary`. A ticket closed with no code says VOID and why, because the selector and the Definition of Done both read this file by id: a ticket that will never be built has to be accounted for here or it is dispatched again every cycle.

- s1-cell-rendertablefit a0bcb37 — utils.Cell + RenderTableFit: weighted, width-budgeted table with escape-safe truncation
- s1-primitives 8c9f314 — presentation Badge/Gauge/RelTime/SectionTitle/ResourceHeader, formatByteCount moved and aliased
- s1-columns 50e891f — presentation.Columns zips two blocks with an escape-preserving cut, stacks below minTwoColWidth
- s2-inpanel-empty 21c5126 — RerenderList writes the muted NoItemsMessage into the side view; messages shortened
- s2-left-profiles-ecs a1c3d7f — SideListPanel gains GetTableCellsFit/Weights; profiles and ECS rows on RenderTableFit, ECS clusters gain a health badge
- s2-left-ec2-s3 d30c315 — EC2 (bold name, muted id) and S3 rows on RenderTableFit; instance id flexes so a narrow panel cannot delete the name
- s2-left-eks-ecr 03f6ea4 — EKS rows keep the status word beside its icon (BadgeCell) and shed the time of day; ECR mutability is an amber badge, starred when an exclusion list makes the policy partial
- s2-left-secrets 401c10f — SecretSummary gains a nil-safe RotationDays from ListSecrets/DescribeSecret; the row reads rotation 7d / rotation on / rotation off
- s2-left-vpc 8ce1d70 — VPC rows on RenderTableFit with a bold CIDR and a muted, content-sized vpc id that stops being cut once the panel has room
- s3-selection-identity 88b0aac — SetItemsKeepSelection re-finds the selected resource by identity after a reload; all 8 side panels wired, and the profile panel's jump to the connected profile is now first-load only
- s3-selection-theme 88b0aac — VOID, nothing built: this gocui's highlighted-line draw never reads View.SelFgColor (forces bright+bold, ORs SelBgColor), measured fg=default with SelFgColor=ColorRed, so the config key and Settings row would have changed nothing
