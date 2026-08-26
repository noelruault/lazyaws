# Backlog — TuiRedesign (tui-redesign)

Build the topmost unbuilt `- [ ]` id not already in `built.md`. ids are **append-only + stable**
(never renumber/delete). Priority order top to bottom. Each must pass the green gate (spec.md).
Every group is reviewed and repaired inside the cycle that built it (spec.md), so nothing else queues.

## Stage 1 — shared render primitives (pure additions, nothing wired)

- [x] `s1-cell-rendertablefit` [d3] — add `utils.Cell{Text, Color}` and `utils.RenderTableFit(rows [][]Cell, width int, weights []int)`: weight 0 sizes to content, >0 shares leftover width, overflow truncated with `…` via runewidth, colorization applied AFTER truncation (truncating ANSI corrupts escape pairs), error on ragged rows like RenderTable. Leave exact-string tests: over-budget column, CJK rune at the cut, weighted remainder absorption, colored cell asserting escapes wrap the truncated text. Do not touch RenderTable (menus/popups keep it).
- [x] `s1-primitives` [d2] — `ui/presentation/primitives.go`: `Badge(status)` (icon+word through the existing statusStyleTable aliases), `Gauge(width, pct)` textual meter clamped 0..100, `RelTime(t, now)` ("6h ago"/"59d ago"), `SectionTitle(s)` cyan heading, `ResourceHeader(kind, name, badge, id, meta...)` three-line inspector header. Move `formatByteCount` from ec2_panel.go into presentation with a thin alias left behind. Exact-string tests using the forceColor helper.
- [x] `s1-columns` [d2] — `ui/presentation/columns.go`: `Columns(width, gap int, left, right string)` zips two pre-rendered blocks side by side with a `│` rule, truncates each line to its column (ANSI-aware measurement via Decolorise), stacks vertically below a `minTwoColWidth` package constant (start 110). Tests: ragged heights, colored lines, narrow fallback, empty right block.

## Stage 2 — left panels, one at a time

- [x] `s2-inpanel-empty` [d2] — `RerenderList` renders the panel's `NoItemsMessage` muted (color.Faint) inside the side view when the list is empty, keeping the existing main-panel message too. Shorten messages to "no EKS clusters" style. Headless test asserting the EKS view buffer.
- [x] `s2-left-profiles-ecs` [d2] — migrate Profiles (verify current cells, no visual change expected) and ECS cluster rows to Cell-based RenderTableFit: `icon  name  N services  R running / P pending  badge`, badge green `● healthy` when Status==ACTIVE and Pending==0, yellow `● deploying` when Pending>0, red `● <status>` otherwise, all from fields already on ECSCluster. Exact-cell tests per state.
- [x] `s2-left-ec2-s3` [d2] — EC2 rows: bold name (or "(no name)"), muted instance id, type, private IP, through RenderTableFit. S3 rows migrated unchanged in content. Exact-cell tests including a 60-char name at width 40.
- [ ] `s2-left-eks-ecr` [d2] — EKS and ECR rows through RenderTableFit; ECR mutability rendered as an amber badge. Exact-cell tests.
- [ ] `s2-left-secrets` [d3] — add rotation cadence to `SecretSummary` mapped from ListSecrets `RotationRules.AutomaticallyAfterDays` (nil-safe: RotationEnabled is ABSENT, not false, on never-rotated secrets); row renders `rotation 7d` / `rotation off` in the right column. Tests: nil rotation, 7d rotation, pending-deletion secret.
- [ ] `s2-left-vpc` [d1] — VPC rows through RenderTableFit with the vpc id muted. Exact-cell tests.

## Stage 3 — selection

- [ ] `s3-selection-theme` [d2] — add `SelectedLineFgColor` to ThemeConfig (default `["bold"]`), apply as `SelFgColor` where SelBgColor is set today, expose a Settings screen row for it. Config default test.
- [ ] `s3-selection-identity` [d3] — `SideListPanel.SetItemsKeepSelection(items, key)` re-finds the previously selected item by identity after reload (index selection jumps when running-first sorts reorder); wire all 8 panel loaders with their natural keys (instance id, cluster ARN, secret name, bucket, repo, vpc id, profile name). Pure table test (reorder/disappear/empty) plus a headless test: select EC2 row, reload with reversed order, same instance still selected.

## Stage 4 — overview machinery and the first inspector

- [ ] `s4-overview-machinery` [d4] HUGE — `Overview` tab plumbing: prepend a MainTab per panel registry, render via NewTickerTask with `Wrap:false`, capture `Views.Main.InnerWidth()` on the UI loop when the task is built, re-render the current tab when main's inner width changes (debounced through the existing 50ms throttle type, guarded by mainBelongsToQ), ticker snapshots `gui.Gen` before each fetch. Interval from `Refresh.OverviewSeconds` (add key, default 2, 0 disables). Headless test: tab list starts with Overview and cycling wraps correctly.
- [ ] `s4-secrets-overview` [d3] HUGE (needs: `s4-overview-machinery`) — Secrets Overview from data the detail path already fetches (DescribeSecret, ListSecretVersionIds include-deprecated, GetResourcePolicy best-effort): ResourceHeader with rotation badge, two-column body (ARN/created/changed/rotated/next/KMS/description/owning-service left; replication, resource policy, tags right), versions table capped at 15 with stage badges. Handle: one version holding AWSCURRENT+AWSPENDING simultaneously, nil stages rendered `-` never `[]`, `Not replicated` when replication nil, `Not configured` when the policy response has no Policy field, AWS-managed secrets (OwningService set), no tags, no description. Pure formatter tests per state.

## Stage 5 — EC2

- [ ] `s5-cw-getmetricdata` [d3] — replace the six serial GetMetricStatistics calls with ONE GetMetricData (30-min window, Period 300, latest datapoint wins, label rendered with the datapoint's own timestamp, never "last 5 minutes"). Empty series (EBS-only instances have no disk metrics) renders `no data`, never 0. Do not rely on response ordering; pick latest by timestamp. Mapper tests: empty series, single series, unsorted timestamps.
- [ ] `s5-ec2-datalayer` [d3] — batch DescribeVolumes into one call for all volume ids (map results by VolumeId, response order differs from request order); cache DescribeInstanceTypes per type on the Client under a mutex (static data: vCPU, memory, network performance). Nil-client guard tests plus mapper tests.
- [ ] `s5-ec2-overview` [d4] HUGE (needs: `s4-overview-machinery`) — `InstanceOverview` aggregate fetched with a WaitGroup fan-out of the existing per-tab fetches, each section independently optional with an `Errs map[string]error` rendered as `<section> unavailable`. Two-column body: Configuration/Network/Metrics left; Status/Storage/Security/Tags right. Storage table includes Throughput when present, encrypted `no` in amber. Console stays OFF the overview (availability unknowable without downloading the full payload; keep it on the Status tab, showing the output's own capture timestamp). Alarms/ASG/EIP stay best-effort and selection-time only, never in tickers. Formatter tests per section including all-none states.

## Stage 6 — ECS

- [ ] `s6-ecs-clusterdata` [d3] — DescribeClusters gains `Include: [STATISTICS, SETTINGS]` in the list path (per-launch-type counts, containerInsights setting); service CPU/memory metrics switch from the ECS/ContainerInsights namespace to plain AWS/ECS with ClusterName+ServiceName dimensions (works regardless of Insights; keep Insights as an additive extra only when the cluster setting is enabled), fetched through the shared GetMetricData path. Mapper tests.
- [ ] `s6-ecs-image` [d3] — resolve the running image from DescribeTasks `containers[].image` (primary container prominent, sidecars summarized `(+1 sidecar)`); when a service has zero running tasks resolve the intended image from the deployment's task definition via the existing memoized path and label it `desired image`. Tests: multi-container, sidecar-only difference, zero tasks.
- [ ] `s6-ecs-cluster-overview` [d4] HUGE (needs: `s4-overview-machinery`) — cluster Overview: health line (status badge, N/M services, running/pending), Configuration (ARN, region, Insights, execute-command), Capacity (providers with explicit `none` state falling back to service launch type), Service Summary table (desired/running/pending/deployment stability: any rolloutState != COMPLETED renders deploying amber, failedTasks>0 red with reason), Tasks table WITH image column, Metrics gauges. Formatter tests: empty capacity providers, rollout IN_PROGRESS, no services.
- [ ] `s6-ecs-service-overview` [d4] HUGE (needs: `s4-overview-machinery`) — service Overview: header with health badge and desired/running/pending, Deployment section (rollout state+reason, circuit breaker, taskdef revision, running image), Networking (subnets, security groups, assignPublicIp), CPU/Memory gauges, recent events. Formatter tests including desired != running.

## Stage 7 — remaining inspectors (reuse existing fetches, zero new AWS calls)

- [ ] `s7-s3-overview` [d3] (needs: `s4-overview-machinery`) — bucket Overview re-laying the existing config-tab data: Security (public access block, encryption, policy present), Data management (versioning, lifecycle, replication, object lock), Access (logging, notifications), Tags. Bucket size stays on-demand only. Formatter tests.
- [ ] `s7-ecr-overview` [d2] (needs: `s4-overview-machinery`) — repository Overview: mutability badge, scan-on-push, encryption, created, latest images table from the existing DescribeImages data. Formatter tests.
- [ ] `s7-vpc-overview` [d3] (needs: `s4-overview-machinery`) — VPC Overview: CIDR, default flag, DNS attributes, subnet counts public/private, IGW/NAT presence, endpoint count, from existing per-tab loaders. Formatter tests.
- [ ] `s7-eks-overview` [d2] (needs: `s4-overview-machinery`) — EKS Overview assembled from the existing details fetch (version, status, endpoint, node groups, addons); formatter-tested only (no live clusters expected), empty state already handled in-panel.

## Stage 8 — auto-refresh engine

- [ ] `s8-adaptive-retry` [d2] — add `awsconfig.WithRetryMode(aws.RetryModeAdaptive)` to baseLoadOptions (client-side rate limiting after throttle responses); keep the logger tests green.
- [ ] `s8-refresh-config` [d2] — add `PanelSeconds` (default 2) and `MetricsSeconds` (default 60, floor 10) to RefreshConfig with Settings screen rows next to OverviewSeconds; defaults test.
- [ ] `s8-refresh-engine` [d4] HUGE (needs: `s3-selection-identity`) — goEvery loop triggering the focused side panel's throttle every PanelSeconds; single-flight per reloader (atomic.Bool, a tick finding the previous reload running is dropped, never queued); overview tickers refetch metrics only when MetricsSeconds elapsed (closure timestamp; GetMetricData is billed per metric requested); on ThrottlingException/RequestLimitExceeded (via smithy.APIError) double that ticker's effective interval up to 60s, decay on success. Tests: single-flight drop, backoff double/decay table, PauseBackgroundThreads respected.

## Stage 9 — polish

- [ ] `s9-copy-key` [d2] — new named key `y` ("copy id/arn") bound per side panel and on main, opening the existing confirmation popup with the full untruncated ID/ARN of the selected item (no clipboard dependency). Keymap conflict tests stay green; regenerate the README key table.
- [ ] `s9-footer-labels` [d1] — align per-panel options-bar labels with the redesign vocabulary (navigate/inspect/copy/refresh/filter/actions/quit), labels only, no key behavior changes.
- [ ] `s9-benchmarks` [d2] — benchmarks with the benchForceColor helper: RenderTableFit (100 rows x 5 cells, width 60), Columns (two 40-line blocks, width 140), each overview formatter with hand-built structs, list rerender with 100 instances. Record results in the handoff; budgets are review guidance, not failing assertions.

## Stage 10 — autonomous UI validation (Playwright over ttyd)

- [ ] `s10-ui-harness` [d4] HUGE — Playwright cannot drive a terminal directly, so bridge it: a `make ui-test` target that starts a fake-AWS endpoint (moto server or LocalStack, wired via `AWS_ENDPOINT_URL`), seeds it with a script (a few EC2 instances incl. one private-only, one ECS cluster+service+running task, S3 buckets, ECR repos, secrets with and without rotation, two VPCs), launches the built TUI inside `ttyd --writable`, and drives the xterm.js page with Playwright: helpers for sendKeys/readScreen/screenshot, non-zero exit on any failed journey. Harness lives under `test/ui` (Node + Playwright, deliberately outside the Go module). Leaves one smoke journey: app boots against the fake endpoint and all 8 panels render.
- [ ] `s10-ui-journeys-panels` [d3] (needs: `s10-ui-harness`) — one journey per left panel: focus via its number key, arrow through rows, assert row content, the selected-row highlight, and in-panel empty states against the seeded data.
- [ ] `s10-ui-journeys-overview` [d3] (needs: `s10-ui-harness`) — per resource: open the Overview tab, assert header and sections render including explicit unavailable/no-data states, cycle tabs with `[` and `]`, resize the terminal narrow and assert the two-column layout stacks instead of wrapping.
- [ ] `s10-ui-journeys-keys` [d3] (needs: `s10-ui-harness`) — filter with `/`, refresh with `r`/`R`, copy popup with `y`, actions menu `a`, options `x`, quit `q`: assert each key does what the footer claims.

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
