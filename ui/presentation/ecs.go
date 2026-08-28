package presentation

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// ecsClusterActive is the one cluster status AWS treats as usable; everything else is a cluster being created, drained or deleted.
const ecsClusterActive = "ACTIVE"

// ecsOverviewTasksShown caps the cluster's tasks table. A busy cluster runs hundreds of tasks against a pane that shows a screenful, and the count of what was left out is what keeps the cap from reading as the whole list.
const ecsOverviewTasksShown = 15

// ecsGaugeWidth sizes the metric bars for the narrowest column a two-column overview gets, so a gauge never decides how wide the column has to be.
const ecsGaugeWidth = 10

// ECSClusterWeights sizes the cluster row so the name absorbs the slack and the counts and badge keep their full text.
func ECSClusterWeights() []int {
	return []int{0, 1, 0, 0, 0}
}

// ECSServiceWeights and ECSTaskWeights mirror ECSClusterWeights for the drilled-in levels, whose rows are a different shape.
func ECSServiceWeights() []int {
	return []int{0, 1, 0, 0}
}

func ECSTaskWeights() []int {
	return []int{0, 1, 0}
}

func GetECSClusterDisplayCells(c *aws.ECSCluster) []utils.Cell {
	return []utils.Cell{
		StatusCellFit(c.Status, StatusStyleIcon),
		{Text: c.Name},
		{Text: fmt.Sprintf("%d services", c.ActiveServicesCount), Color: color.FgYellow},
		{Text: fmt.Sprintf("%d running / %d pending", c.RunningTasksCount, c.PendingTasksCount)},
		ecsClusterBadge(c),
	}
}

// ecsClusterBadge answers "is this cluster fine right now" in one glance, which the raw status word cannot: a cluster is ACTIVE while its tasks are still coming up.
// Only an ACTIVE cluster can be healthy or deploying — pending tasks on a cluster that is being deleted are draining, not rolling out, and calling that "deploying" would read as progress.
func ecsClusterBadge(c *aws.ECSCluster) utils.Cell {
	switch {
	case c.Status == ecsClusterActive && c.PendingTasksCount == 0:
		return utils.Cell{Text: "● healthy", Color: color.FgGreen}
	case c.Status == ecsClusterActive:
		return utils.Cell{Text: "● deploying", Color: color.FgYellow}
	case c.Status == "":
		// DescribeClusters omits the status of a cluster it could not read; "● " alone would look like a rendering bug.
		return utils.Cell{Text: "● unknown", Color: color.FgRed}
	default:
		return utils.Cell{Text: "● " + c.Status, Color: color.FgRed}
	}
}

func GetECSServiceDisplayCells(s *aws.ECSService) []utils.Cell {
	return []utils.Cell{
		StatusCellFit(s.Status, StatusStyleIcon),
		{Text: s.Name},
		{Text: s.LaunchType, Color: color.FgYellow},
		{Text: fmt.Sprintf("%d/%d", s.RunningCount, s.DesiredCount)},
	}
}

// GetECSServiceDisplayStrings is the service row already coloured, for the cluster inspector's services table, which still lays out with RenderTable.
func GetECSServiceDisplayStrings(s *aws.ECSService) []string {
	cells := GetECSServiceDisplayCells(s)
	texts := make([]string, len(cells))
	for i, cell := range cells {
		texts[i] = cell.Rendered()
	}

	return texts
}

// ECSImageSummary names the image a service identifies with and says how many containers ride alongside it, without listing them: the sidecar images are on the task drill level, and repeating them here would push the one that matters out of the pane.
func ECSImageSummary(image aws.ECSServiceImage) string {
	if image.Image == "" {
		return "unavailable"
	}
	if image.Sidecars <= 0 {
		return image.Image
	}
	if image.Sidecars == 1 {
		return image.Image + " (+1 sidecar)"
	}
	return fmt.Sprintf("%s (+%d sidecars)", image.Image, image.Sidecars)
}

// ECSImageLabel distinguishes an image read off a running container from one read off a task definition, because a service with nothing running still has an intended image and reporting it as running would be a lie the pane cannot walk back.
func ECSImageLabel(image aws.ECSServiceImage) string {
	if image.Desired {
		return "Desired image"
	}
	return "Running image"
}

// FormatECSClusterOverview lays a cluster out for the Overview tab: a header that always renders, then the two-column body the Config, Instances and Tags tabs are consolidated into.
// The header is built from the LIST ROW rather than from the fetch, so a cluster whose every section failed is still identified and still carries the badge the side panel shows it with.
func FormatECSClusterOverview(c *aws.ECSCluster, o *aws.ECSClusterOverview, width int) string {
	// The meta line is the region alone: the services summary and the task counts live in the stat cards beside it, and a meta line repeating a card is the first thing the cards' width truncates.
	header := HeaderWithStats(width,
		ResourceHeader("ECS Cluster", c.Name, ecsClusterBadge(c).Rendered(), "", c.Region),
		clusterStatCards(c, o),
	)

	column := ColumnWidth(width, overviewGap)
	left := joinBlocks(
		clusterHealthBlock(c, o),
		clusterConfigBlock(c),
		clusterMetricsBlock(o),
	)
	right := joinBlocks(
		clusterServicesBlock(o, column),
		clusterCapacityBlock(c, o),
		clusterTasksBlock(o, column),
		clusterTagsBlock(o, column),
	)

	return header + "\n\n" + Columns(width, overviewGap, left, right)
}

// clusterServiceHealth reduces the service list to the numbers the Health section and the header cards read: how many services hold their desired count, whether any rollout is still open, and how many tasks the open ones have lost.
func clusterServiceHealth(o *aws.ECSClusterOverview) (steady int, rolling bool, failed int32) {
	for i := range o.Services {
		s := &o.Services[i]
		if ecsServiceIsSteady(s) {
			steady++
		}
		for _, d := range s.Deployments {
			failed += d.FailedTasks
			if !ecsDeploymentSettled(d) {
				rolling = true
			}
		}
	}

	return steady, rolling, failed
}

// clusterStatCards is the header's compact stat row. Values keep the palette's meaning: green is holding, amber is moving, plain is zero.
func clusterStatCards(c *aws.ECSCluster, o *aws.ECSClusterOverview) []Stat {
	services := utils.Cell{Text: fmt.Sprintf("%d", c.ActiveServicesCount)}
	if o.Err(aws.SectionServices) == nil {
		steady, _, _ := clusterServiceHealth(o)
		services = utils.Cell{Text: fmt.Sprintf("%d / %d", steady, len(o.Services))}
		if steady == len(o.Services) && len(o.Services) > 0 {
			services.Color = color.FgGreen
		}
	}

	tasks := utils.Cell{Text: fmt.Sprintf("%d running", c.RunningTasksCount)}
	if c.RunningTasksCount > 0 {
		tasks.Color = color.FgGreen
	}

	pending := utils.Cell{Text: fmt.Sprintf("%d", c.PendingTasksCount)}
	if c.PendingTasksCount > 0 {
		pending.Color = color.FgYellow
	}

	return []Stat{
		{Label: "Services", Value: services},
		{Label: "Tasks", Value: tasks},
		{Label: "Pending", Value: pending},
	}
}

// clusterHealthBlock carries only what no card already says: the deployment verdict with its failed-task count, and what the cluster runs on.
// It shrank from the mockups' card grid on the owner's dedup rule (2026-08-28): the cluster status echoed the header badge and the service and task counts echoed the header cards, so the cards here were the same numbers in a second frame.
func clusterHealthBlock(c *aws.ECSCluster, o *aws.ECSClusterOverview) string {
	deployments := utils.ColoredString("unavailable", color.FgRed)
	if err := o.Err(aws.SectionServices); err != nil {
		deployments = fieldOr(err, "")
	} else {
		_, rolling, failed := clusterServiceHealth(o)
		switch {
		case failed > 0:
			deployments = utils.ColoredString(fmt.Sprintf("%d failed tasks", failed), color.FgRed)
		case rolling:
			deployments = utils.ColoredString("deploying", color.FgYellow)
		default:
			deployments = utils.ColoredString("stable", color.FgGreen)
		}
	}

	return SectionTitle("Health") + "\n" + kvBlock([]kv{
		{"Deployments", deployments},
		// 0 is the normal Fargate answer, not a problem.
		{"Container instances", fmt.Sprintf("%d", c.RegisteredContainerCount)},
	})
}

func clusterTagsBlock(o *aws.ECSClusterOverview, width int) string {
	if err := o.Err(aws.SectionTags); err != nil {
		return sectionUnavailable("Tags", err)
	}
	if len(o.Tags) == 0 {
		return SectionTitle("Tags") + "\nnone"
	}

	return SectionTitle("Tags") + "\n" + tagsBodyFrom(width, o.Tags)
}

// ecsServiceIsSteady is the service-level twin of the cluster badge: it holds its desired count and has no rollout still open.
func ecsServiceIsSteady(s *aws.ECSService) bool {
	if s.RunningCount != s.DesiredCount {
		return false
	}
	for _, d := range s.Deployments {
		if !ecsDeploymentSettled(d) {
			return false
		}
	}

	return true
}

// ecsDeploymentSettled treats an EMPTY rollout state as settled, which is the one case worth spelling out: ECS omits rolloutState entirely for a CODE_DEPLOY or EXTERNAL controller.
// Reading absent as "not COMPLETED" would leave every blue/green service permanently amber, which spends the colour on the services that need it least.
func ecsDeploymentSettled(d aws.ECSDeployment) bool {
	return d.RolloutState == "" || d.RolloutState == aws.ECSRolloutCompleted
}

func clusterConfigBlock(c *aws.ECSCluster) string {
	return SectionTitle("Configuration") + "\n" + kvBlock([]kv{
		{"ARN", orNone(c.Arn)},
		{"Region", orNone(c.Region)},
		{"Container Insights", clusterInsightsValue(c.ContainerInsights)},
		{"Execute command", executeCommandValue(c.ExecuteCommandLogging)},
		{"Console", orNone(c.ConsoleURL)},
	})
}

// clusterInsightsValue keeps an empty setting as unknown rather than folding it into disabled, because the two call for different actions: one is a cluster to switch Insights on for, the other is a describe call that did not ask for the field.
func clusterInsightsValue(setting string) string {
	if setting == "" {
		return "unknown"
	}

	return setting
}

// executeCommandValue says where a session's output actually goes, which the setting name alone does not: DEFAULT means the task's own awslogs driver, and NONE means sessions run but are recorded nowhere.
func executeCommandValue(logging string) string {
	switch logging {
	case "":
		return "not configured"
	case "NONE":
		return "NONE (sessions not logged)"
	case "DEFAULT":
		return "DEFAULT (task awslogs driver)"
	case "OVERRIDE":
		return "OVERRIDE (own log configuration)"
	default:
		return logging
	}
}

// clusterCapacityBlock says what the cluster places tasks on, and where there are no capacity providers says what the services use instead.
// A cluster with none is not a cluster without capacity: it launches on a bare LaunchType, and a lone "none" reads as broken rather than as configured differently.
// The container instance count moved to the Health section's grid, where the mockups keep it.
func clusterCapacityBlock(c *aws.ECSCluster, o *aws.ECSClusterOverview) string {
	title := SectionTitle("Capacity")

	if len(c.DefaultCapacityProviderStrat) > 0 {
		lines := make([]string, 0, len(c.DefaultCapacityProviderStrat)+1)
		lines = append(lines, title)
		for _, s := range c.DefaultCapacityProviderStrat {
			lines = append(lines, fmt.Sprintf("  %s (base %d, weight %d)", s.CapacityProvider, s.Base, s.Weight))
		}

		return strings.Join(lines, "\n")
	}

	// Providers attached with no default strategy is its own state: a service then has to name a provider itself, and one that names none is never placed.
	if len(c.CapacityProviders) > 0 {
		return title + "\n  " + strings.Join(c.CapacityProviders, ", ") +
			"\n  " + utils.ColoredString("no default strategy", color.Faint)
	}

	return title + "\n  none, " + clusterLaunchTypes(o)
}

// clusterLaunchTypes reads what a cluster with no capacity providers actually runs on off the services, rather than assuming Fargate.
func clusterLaunchTypes(o *aws.ECSClusterOverview) string {
	if o.Err(aws.SectionServices) != nil {
		return "services unavailable"
	}

	launchTypes := make([]string, 0, 2)
	for _, s := range o.Services {
		if s.LaunchType != "" && !slices.Contains(launchTypes, s.LaunchType) {
			launchTypes = append(launchTypes, s.LaunchType)
		}
	}
	if len(launchTypes) == 0 {
		return "no services to place"
	}
	slices.Sort(launchTypes)

	return "services launch on " + strings.Join(launchTypes, ", ")
}

func clusterMetricsBlock(o *aws.ECSClusterOverview) string {
	title := SectionTitle("Metrics")

	// A cluster with Insights off is not a cluster CloudWatch failed to answer for, and rendering both as "no data" hides the one that is a setting away from having numbers.
	// Kept short enough to survive the narrowest two-column pane, where a longer sentence is cut exactly where it says why.
	if o.InsightsOff {
		return title + "\n" + utils.ColoredString("Container Insights off, no metrics", color.Faint)
	}
	if err := o.Err(aws.SectionMetrics); err != nil {
		return sectionUnavailable("Metrics", err)
	}

	m := o.Metrics

	return title + "\n" + kvBlock([]kv{
		clusterGaugeRow("CPU", "units", m.CPUUsed, m.CPUReserved),
		clusterGaugeRow("Memory", "MiB", m.MemUsed, m.MemReserved),
	})
}

// clusterGaugeRow pairs the bar with the absolute readings behind it, because a percentage on its own cannot tell a small busy cluster from a large idle one.
func clusterGaugeRow(label, unit string, used, reserved aws.MetricPoint) kv {
	pct, ok := aws.UtilizationPercent(used, reserved)
	if !ok {
		return kv{label, "no data"}
	}

	return kv{label, fmt.Sprintf("%s  %.0f / %.0f %s", Gauge(ecsGaugeWidth, pct), used.Value, reserved.Value, unit)}
}

func clusterServicesBlock(o *aws.ECSClusterOverview, width int) string {
	if err := o.Err(aws.SectionServices); err != nil {
		return sectionUnavailable("Service Summary", err)
	}

	if len(o.Services) == 0 {
		return SectionTitle("Service Summary") + "\nno services"
	}
	title := SectionTitleWithNote(width, "Service Summary", pluralize(len(o.Services), "service"))

	// Sorted on a copy: the pane re-renders on a ticker, and rows that follow the order ListServices happened to answer in would reshuffle under the cursor between refreshes.
	services := slices.Clone(o.Services)
	slices.SortFunc(services, func(a, b aws.ECSService) int { return strings.Compare(a.Name, b.Name) })

	rows := make([][]utils.Cell, len(services))
	for i := range services {
		running := utils.Cell{Text: fmt.Sprintf("%d", services[i].RunningCount)}
		// Green only when the service holds what it asked for; a shortfall stays plain and lets the status badge carry the alarm.
		if services[i].RunningCount == services[i].DesiredCount && services[i].DesiredCount > 0 {
			running.Color = color.FgGreen
		}
		rows[i] = []utils.Cell{
			{Text: services[i].Name, Color: color.Bold},
			{Text: fmt.Sprintf("%d", services[i].DesiredCount)},
			running,
			{Text: fmt.Sprintf("%d", services[i].PendingCount)},
			ecsServiceStabilityCell(&services[i]),
		}
	}
	// Only the name has no natural width, so it is the one column that flexes and the counts and badge keep their full text.
	table := BoxedTable(width, []int{1, 0, 0, 0, 0}, []string{"Service", "Desired", "Running", "Pending", "Status"}, rows)

	lines := []string{title, table}
	for i := range services {
		if reason := ecsServiceRolloutReason(&services[i]); reason != "" {
			lines = append(lines, utils.ColoredString("  "+services[i].Name+": "+reason, color.FgRed))
		}
	}

	return strings.Join(lines, "\n")
}

// ecsServiceStabilityCell reduces a service's deployments to the one thing worth a glance, and never reports steady while a rollout is still open.
// Lost tasks outrank a rollout still in progress: ECS retries a failed task, so an IN_PROGRESS deployment with failures can sit amber forever while it is really stuck.
func ecsServiceStabilityCell(s *aws.ECSService) utils.Cell {
	var failed int32
	rolling, rolloutFailed := false, false
	for _, d := range s.Deployments {
		failed += d.FailedTasks
		if d.RolloutState == aws.ECSRolloutFailed {
			rolloutFailed = true
		}
		if !ecsDeploymentSettled(d) {
			rolling = true
		}
	}

	switch {
	case failed > 0:
		return utils.Cell{Text: fmt.Sprintf("● %d failed", failed), Color: color.FgRed}
	case rolloutFailed:
		return utils.Cell{Text: "● failed", Color: color.FgRed}
	case rolling:
		return utils.Cell{Text: "● deploying", Color: color.FgYellow}
	case s.RunningCount != s.DesiredCount:
		return utils.Cell{Text: "● scaling", Color: color.FgYellow}
	default:
		return utils.Cell{Text: "● steady", Color: color.FgGreen}
	}
}

// ecsServiceRolloutReason is the sentence ECS gives for a rollout going wrong, and the only thing on the pane that says WHY.
// It gets a line of its own because it is a sentence: in a table cell it would be cut to its first few words, which are the least specific part of it.
func ecsServiceRolloutReason(s *aws.ECSService) string {
	for _, d := range s.Deployments {
		if d.RolloutStateReason == "" {
			continue
		}
		if d.FailedTasks > 0 || d.RolloutState == aws.ECSRolloutFailed {
			return d.RolloutStateReason
		}
	}

	return ""
}

func clusterTasksBlock(o *aws.ECSClusterOverview, width int) string {
	if err := o.Err(aws.SectionTasks); err != nil {
		return sectionUnavailable("Tasks", err)
	}

	title := SectionTitle("Tasks")
	if len(o.Tasks) == 0 {
		return title + "\nno running tasks"
	}

	// Sorted on a copy for the same reason the services are: this pane re-renders on a ticker and DescribeTasks promises no order.
	tasks := slices.Clone(o.Tasks)
	slices.SortFunc(tasks, func(a, b aws.ECSTask) int { return strings.Compare(a.ID, b.ID) })

	shown := min(len(tasks), ecsOverviewTasksShown)
	rows := make([][]utils.Cell, shown)
	for i, t := range tasks[:shown] {
		rows[i] = []utils.Cell{
			{Text: t.ID, Color: color.Faint},
			{Text: orNone(taskDefRef(t.TaskDefinition))},
			StatusCellFit(t.Status, StatusStyleLong),
			{Text: ecsTaskImageText(t)},
		}
	}
	// The mockups' task table plus the image column, which stays: showing the image a task actually runs is a standing requirement of this pane.
	// The image is the column with no natural width, so it carries the weight.
	table := BoxedTable(width, []int{0, 0, 0, 1}, []string{"Task", "Revision", "Status", "Image"}, rows)

	lines := []string{title, table}
	if hidden := len(tasks) - shown; hidden > 0 {
		lines = append(lines, utils.ColoredString(fmt.Sprintf("(%d more)", hidden), color.Faint))
	}

	return strings.Join(lines, "\n")
}

// ecsTaskImageText names what one task is running, which is the hard requirement this pane exists for; a task whose containers could not be read says so rather than leaving the column blank.
func ecsTaskImageText(t aws.ECSTask) string {
	image, ok := aws.ECSTaskImage(t)
	if !ok {
		return "unavailable"
	}

	return ECSImageSummary(image)
}

// ecsServiceEventsShown caps the events list. ECS keeps the last hundred, the pane has room for a handful, and the ones worth a glance are the newest.
const ecsServiceEventsShown = 5

// ecsEventAgeWidth pads the relative time so the messages line up in a column the eye can run down.
// It is a minimum rather than a cut: an age wider than this (a four-digit day count) pushes its own message right instead of losing digits, which costs one ragged line and no information.
const ecsEventAgeWidth = 8

// ecsServiceMetricStat captions a service reading with the window it was averaged over, matching the period the ECS metric queries ask for.
const ecsServiceMetricStat = "1-min avg"

// FormatECSServiceOverview lays a service out for the Overview tab: a header that always renders, then the two-column body its Config, Deployments and Events tabs are consolidated into.
// The header is built from the LIST ROW rather than from the fetch, because everything in it arrives with DescribeServices: a service whose metrics and image both failed is still identified and still carries its stability badge.
func FormatECSServiceOverview(s *aws.ECSService, o *aws.ECSServiceOverview, width int, now time.Time) string {
	// Cut to the pane: the header spans the full width rather than a column, so Columns never sees it, and with wrap off a long name plus its badge and counts runs off the edge unmarked.
	header := truncateBlock(ResourceHeader("Service", s.Name, ecsServiceStabilityCell(s).Rendered(), "",
		s.Cluster,
		s.LaunchType,
		fmt.Sprintf("%d desired / %d running / %d pending", s.DesiredCount, s.RunningCount, s.PendingCount),
	), width)

	left := joinBlocks(
		serviceDeploymentBlock(s, o, now),
		serviceNetworkBlock(s),
	)
	right := joinBlocks(
		serviceMetricsBlock(o),
		serviceScalingBlock(o),
		serviceEventsBlock(s, now),
	)

	return header + "\n\n" + Columns(width, overviewGap, left, right)
}

// scalingMetricSuffix names the tracked metric beside its target when the policy reports one.
func scalingMetricSuffix(metric string) string {
	if metric == "" {
		return ""
	}
	return " (" + metric + ")"
}

// serviceScalingBlock folds the old Scaling tab in: bounds and policies fit a section, and a service with none says so in one line.
func serviceScalingBlock(o *aws.ECSServiceOverview) string {
	if err := o.Err(aws.SectionScaling); err != nil {
		return sectionUnavailable("Scaling", err)
	}
	scaling := o.Scaling
	if scaling == nil {
		return SectionTitle("Scaling") + "\nnot registered"
	}

	lines := []string{SectionTitle("Scaling"), kvBlock([]kv{
		{"Capacity", fmt.Sprintf("%d - %d tasks", scaling.MinCapacity, scaling.MaxCapacity)},
	})}
	if len(scaling.Policies) == 0 {
		lines = append(lines, "Policies:\n  none")
	} else {
		lines = append(lines, "Policies:")
	}
	for _, p := range scaling.Policies {
		lines = append(lines, "  "+p.Name+" "+utils.ColoredString(p.Type, color.Faint))
		switch p.Type {
		case "TargetTrackingScaling":
			lines = append(lines, fmt.Sprintf("    target %.1f%s, cooldown in %ds / out %ds",
				p.TargetValue, scalingMetricSuffix(p.TargetMetric), p.ScaleInCooldownSecs, p.ScaleOutCooldownSecs))
		case "StepScaling":
			lines = append(lines, fmt.Sprintf("    %d step adjustment(s), cooldown %ds", p.StepAdjustments, p.ScaleOutCooldownSecs))
		}
	}

	return strings.Join(lines, "\n")
}

// primaryECSDeployment is the deployment the service is trying to reach; the others are ones it is draining away from, and reporting their rollout state would describe a deployment already being replaced.
// A service between deployments has none at all, and the zero value renders as a rollout no controller reported rather than as one that failed.
func primaryECSDeployment(s *aws.ECSService) aws.ECSDeployment {
	for _, d := range s.Deployments {
		if d.Status == aws.ECSDeploymentPrimary {
			return d
		}
	}
	if len(s.Deployments) > 0 {
		return s.Deployments[0]
	}

	return aws.ECSDeployment{}
}

func serviceDeploymentBlock(s *aws.ECSService, o *aws.ECSServiceOverview, now time.Time) string {
	primary := primaryECSDeployment(s)

	lines := []string{SectionTitle("Deployment"), kvBlock([]kv{
		{"Controller", orNone(s.DeploymentController)},
		{"Rollout", ecsRolloutValue(primary)},
		{"Started", deploymentStarted(primary, now)},
		{"Circuit breaker", circuitBreakerValue(s)},
		{"Task definition", orNone(taskDefRef(aws.ServiceTaskDefinition(s)))},
		serviceImageRow(o),
	})}

	// The reason is a sentence, so it gets a line of its own: in a value cell it would be cut to its first few words, which are the least specific part of it.
	if primary.RolloutStateReason != "" {
		lines = append(lines, utils.ColoredString(primary.RolloutStateReason, color.FgRed))
	}

	return strings.Join(lines, "\n")
}

// ecsRolloutValue reports the rollout as a badge, and counts the tasks it lost alongside it: ECS retries a failed task, so a deployment can sit at IN_PROGRESS indefinitely and the count is the difference between slow and stuck.
// An EMPTY state is "not reported" rather than a failure, because ECS omits rolloutState entirely for a CODE_DEPLOY or EXTERNAL controller.
func ecsRolloutValue(d aws.ECSDeployment) string {
	if d.RolloutState == "" {
		return "not reported"
	}
	if d.FailedTasks > 0 {
		return Badge(d.RolloutState) + utils.ColoredString(fmt.Sprintf("  %d failed", d.FailedTasks), color.FgRed)
	}

	return Badge(d.RolloutState)
}

// deploymentStarted is how long the current deployment has been going, which is what turns an IN_PROGRESS rollout into either "still rolling" or "stuck".
func deploymentStarted(d aws.ECSDeployment, now time.Time) string {
	if d.Created == nil {
		return "unknown"
	}

	return RelTime(*d.Created, now)
}

// circuitBreakerValue says whether a failed rollout is left standing or wound back, because "enabled" alone does not: the breaker stops a bad deployment either way, and only rollback restores the previous one.
func circuitBreakerValue(s *aws.ECSService) string {
	switch {
	case !s.CircuitBreakerEnabled:
		return "disabled"
	case s.CircuitBreakerRollback:
		return "enabled, rolls back"
	default:
		return "enabled, no rollback"
	}
}

// taskDefRef keeps family:revision and drops the ARN prefix, which is the account and region the pane already states.
func taskDefRef(taskDefArn string) string {
	if idx := strings.LastIndex(taskDefArn, "/"); idx != -1 {
		return taskDefArn[idx+1:]
	}

	return taskDefArn
}

// serviceImageRow labels the image running or desired, which is the distinction spec's ECS requirement turns on, and keeps a failed resolution on the pane with its reason rather than as a blank.
// A failed fetch is labelled neutrally: with nothing resolved there is no telling which of the two it would have been.
func serviceImageRow(o *aws.ECSServiceOverview) kv {
	if err := o.Err(aws.SectionImage); err != nil {
		return kv{"Image", utils.ColoredString("unavailable: "+err.Error(), color.FgRed)}
	}

	return kv{ECSImageLabel(o.Image), ECSImageSummary(o.Image)}
}

func serviceNetworkBlock(s *aws.ECSService) string {
	title := SectionTitle("Networking")

	// No configuration is not a service whose networking failed to load: ECS requires the block for awsvpc and rejects it for every other mode, so its absence says which mode the task definition uses.
	if s.Network == nil {
		return title + "\n" + utils.ColoredString("no awsvpc configuration", color.Faint)
	}

	lines := []string{title, kvBlock([]kv{{"Public IP", publicIPValue(s.Network.AssignPublicIP)}})}
	lines = append(lines, idListBlock("Subnets", s.Network.Subnets)...)
	lines = append(lines, idListBlock("Security groups", s.Network.SecurityGroups)...)

	return strings.Join(lines, "\n")
}

// publicIPValue never fills in a missing flag: the default depends on how the service was created, and rendering DISABLED for an unanswered field is a claim about reachability.
func publicIPValue(assign string) string {
	if assign == "" {
		return "not reported"
	}

	return assign
}

// idListBlock puts each identifier on its own line rather than joining them, because a column narrow enough to cut a joined list drops the ids at the end of it with only an ellipsis to show for them.
func idListBlock(label string, ids []string) []string {
	if len(ids) == 0 {
		return []string{utils.ColoredString(label+":", color.Faint) + " none"}
	}

	lines := make([]string, 0, len(ids)+1)
	lines = append(lines, utils.ColoredString(fmt.Sprintf("%s (%d):", label, len(ids)), color.FgYellow))
	for _, id := range ids {
		lines = append(lines, "  "+id)
	}

	return lines
}

func serviceMetricsBlock(o *aws.ECSServiceOverview) string {
	if err := o.Err(aws.SectionMetrics); err != nil {
		return sectionUnavailable("Metrics", err)
	}

	// Read through a zero value rather than guarded per row: a hand-built overview with no metrics and a fetch that answered with nothing published are the same thing on screen, and neither is an error.
	var cpu, mem aws.MetricPoint
	if o.Metrics != nil {
		cpu, mem = o.Metrics.CPUUtilization, o.Metrics.MemoryUtilization
	}

	return SectionTitle("Metrics") + "\n" + kvBlock([]kv{
		serviceGaugeRow("CPU", cpu),
		serviceGaugeRow("Memory", mem),
	})
}

// serviceGaugeRow draws a bar only from a reading CloudWatch published: an unpublished series and a genuinely idle service both compute to 0%, and a bar sitting at zero is the more believable of the two.
func serviceGaugeRow(label string, p aws.MetricPoint) kv {
	if !p.OK {
		return kv{label, "no data"}
	}

	return kv{label, fmt.Sprintf("%s  (%s @ %s)", Gauge(ecsGaugeWidth, p.Value), ecsServiceMetricStat, p.At.UTC().Format("15:04Z"))}
}

// serviceEventsBlock is what says WHY a service is not where it wants to be, which no count on this pane can.
func serviceEventsBlock(s *aws.ECSService, now time.Time) string {
	title := SectionTitle("Recent events")
	if len(s.Events) == 0 {
		return title + "\nno recent events"
	}

	// Sorted newest first on a copy: DescribeServices answers most-recent-first in practice but promises no order, and this pane re-renders on a ticker where a reshuffle would be read as new events arriving.
	events := slices.Clone(s.Events)
	slices.SortStableFunc(events, func(a, b aws.ECSEvent) int { return eventTime(b).Compare(eventTime(a)) })

	shown := min(len(events), ecsServiceEventsShown)
	lines := make([]string, 0, shown+1)
	lines = append(lines, title)
	for _, ev := range events[:shown] {
		age := utils.WithPadding(utils.ColoredString(eventAge(ev, now), color.Faint), ecsEventAgeWidth)
		lines = append(lines, age+" "+ecsEventMessage(s.Name, ev.Message))
	}

	return strings.Join(lines, "\n")
}

func eventTime(ev aws.ECSEvent) time.Time {
	if ev.When == nil {
		return time.Time{}
	}

	return *ev.When
}

func eventAge(ev aws.ECSEvent, now time.Time) string {
	return RelTime(eventTime(ev), now)
}

// ecsEventMessage drops the "(service <name>) " ECS opens every message with, which repeats the pane's own header and costs a fifth of a narrow column.
// The prefix is matched exactly, for THIS service, so a message in any other shape is left whole rather than cut on a guess about the format.
func ecsEventMessage(serviceName, message string) string {
	return strings.TrimPrefix(message, "(service "+serviceName+") ")
}

func GetECSTaskDisplayCells(t *aws.ECSTask) []utils.Cell {
	return []utils.Cell{
		StatusCellFit(t.Status, StatusStyleIcon),
		{Text: t.ID, Color: color.FgMagenta},
		{Text: t.LaunchType},
	}
}
