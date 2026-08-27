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
- s4-overview-machinery 74fbfe3 — Overview tab plumbing: a width-aware task built on the UI loop, wrap off, generation-guarded, re-rendered when main resizes; refresh.overviewSeconds (default 2, 0 off); prepended to the seven resource registries
- s4-secrets-overview 7226b1a — Secrets Overview from the metadata the Config tab already fetches: rotation badge, two-column body, capped version table with joint and absent stages; Columns stacked path now cuts to the pane (265d1a0)
- s5-cw-getmetricdata 649fb8b — six EC2 metrics in one GetMetricData, results matched by query id, each reading stamped with its own timestamp and an unpublished series rendering "no data" instead of 0
- s5-ec2-datalayer 6fd562c — one DescribeVolumes for every attached volume matched back by VolumeId, and DescribeInstanceTypes cached per type on the Client
- s5-ec2-overview 85f4366 — Overview tab for EC2: a WaitGroup fan-out whose sections fail independently, two columns of seven sections, and the alarm/ASG/address lookups priced per selection instead of per tick
- s6-ecs-clusterdata b8d57d7 — DescribeClusters carries STATISTICS+SETTINGS, and service CPU/memory moves to AWS/ECS through GetMetricData with the Insights reservations as gated extras in the same call
- s6-ecs-image ff71d1d — the running image comes off DescribeTasks' newest running task with its sidecars counted, falling back to the PRIMARY deployment's memoized task definition labelled desired
- s6-ecs-cluster-overview 5dc0f7b — cluster Overview: health header, Configuration, Capacity falling back to the service launch type, Metrics gauges from one GetMetricData, a Service Summary carrying rollout stability, and a Tasks table with the running image
- s6-ecs-service-overview fa48f80 — service Overview tab: stability badge with desired/running/pending, Deployment (rollout state+reason, circuit breaker, taskdef revision, running image), awsvpc networking, CPU/Memory gauges and recent events
- s7-s3-overview d8fc7ae — bucket Overview from the Config tab's eleven calls: public-access posture as the header badge, per-fetch failure isolation, size left on demand; renders once per selection
- s7-ecr-overview 643c89b — repository Overview off the list row plus one DescribeImages: mutability badge, policies with the lifecycle's last evaluation, and a latest-images table the formatter sorts itself
- s7-vpc-overview 6acb327 — VPC Overview consolidating Config/Subnets/Gateways/Endpoints: public split by routing not by auto-assign, DNS as three answers, endpoints counted by type
- s7-eks-overview d3e11d0 — EKS cluster Overview from the three loaders its tabs already call; Configuration reads the list row so version/status/endpoint survive a denied describe, node-group version drift is marked, and public CIDRs are withheld while the public endpoint is off
- s8-adaptive-retry f3d33c5 — every client on retry.NewAdaptiveMode: set in baseLoadOptions AND in newClientFromConfig, because the cached-credentials path never calls LoadDefaultConfig and is the path normal operation takes
- s8-refresh-config d2f3c3a — RefreshConfig gains PanelSeconds (2) and MetricsSeconds (60, floor applied on read); Settings grows an interval row kind writing !!int, with rows for all three tiers
- s8-refresh-engine 664034b — the focused panel on a 2s tier behind a single-flight guard, CloudWatch metrics on a per-resource memo at 60s, and a throttled overview dropping ticks to double its effective interval up to 60s
- s9-copy-key 8b3321f — new key `y` on every list and on main, showing the selected row's full id or ARN in the existing confirmation popup; each panel publishes its own copy value (ARN where the list call answers one), resolved through focus history so it works from the detail pane
- s9-footer-labels 89519e3 — the options bar is built per focused view in the redesign's vocabulary, ordered by frequency instead of alphabetically by keycap, with every rebindable label read from the keymap and an 87-cell worst case under a tested 90-cell budget
- s9-benchmarks e5c7f74 — benchmarks for the fit table (with RenderTable beside it for the ratio), the column zipper both layouts, all eight overview formatters and the 100-instance list rerender; `make bench` grew the two new packages and a test pins that list against the tree
- s10-ui-harness 628c034 — make ui-test: moto in docker, seeded fixtures, the real TUI under ttyd, driven from Chromium; smoke journey asserts all eight panels render
- s10-ui-journeys-panels 16ee5c3 — one journey per left panel: number key, rows as whole lines, and the selected-row highlight read out of the cell attributes because SelBgColor is all this gocui draws
- s10-ui-journeys-overview f92a213 — six resources' Overview tabs asserted whole (tab order, header, sections), two columns at 1700px and stacked at 1000px, with readScreen reading exactly cols cells so xterm's reflow stops faking a layout
- s10-ui-journeys-keys a6a9e62 — every key the footer advertises checked against what it does; R proved by a bucket the unfocused panel cannot see any other way, r documented as inseparable from its own refresh tier
