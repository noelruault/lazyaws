package presentation

import (
	"fmt"
	"slices"
	"strings"

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
	// Cut to the pane: the header spans the full width rather than a column, so Columns never sees it, and with wrap off an over-long meta line runs off the edge unmarked.
	header := truncateBlock(ResourceHeader("Cluster", c.Name, ecsClusterBadge(c).Rendered(), "",
		c.Region,
		clusterServicesSummary(c, o),
		fmt.Sprintf("%d running / %d pending", c.RunningTasksCount, c.PendingTasksCount),
	), width)

	column := ColumnWidth(width, overviewGap)
	left := joinBlocks(
		clusterConfigBlock(c),
		clusterCapacityBlock(c, o),
		clusterMetricsBlock(o),
	)
	right := joinBlocks(
		clusterServicesBlock(o, column),
		clusterTasksBlock(o, column),
	)

	return header + "\n\n" + Columns(width, overviewGap, left, right)
}

// clusterServicesSummary counts the services actually holding what they were asked to run, which the cluster's own ActiveServicesCount cannot say: a service stays ACTIVE while every task it wants is failing to start.
// Only the service list carries that, so a failed services fetch falls back to the count the cluster itself reported rather than dropping the line out of the header.
func clusterServicesSummary(c *aws.ECSCluster, o *aws.ECSClusterOverview) string {
	if o.Err(aws.SectionServices) != nil {
		return fmt.Sprintf("%d services", c.ActiveServicesCount)
	}
	if len(o.Services) == 0 {
		return "no services"
	}

	steady := 0
	for i := range o.Services {
		if ecsServiceIsSteady(&o.Services[i]) {
			steady++
		}
	}

	return fmt.Sprintf("%d/%d services steady", steady, len(o.Services))
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
		return sectionUnavailable("Services", err)
	}

	title := SectionTitle("Services")
	if len(o.Services) == 0 {
		return title + "\nno services"
	}

	// Sorted on a copy: the pane re-renders on a ticker, and rows that follow the order ListServices happened to answer in would reshuffle under the cursor between refreshes.
	services := slices.Clone(o.Services)
	slices.SortFunc(services, func(a, b aws.ECSService) int { return strings.Compare(a.Name, b.Name) })

	rows := make([][]utils.Cell, len(services))
	for i := range services {
		rows[i] = []utils.Cell{
			{Text: services[i].Name, Color: color.Bold},
			{Text: fmt.Sprintf("%d/%d running", services[i].RunningCount, services[i].DesiredCount)},
			{Text: fmt.Sprintf("%d pending", services[i].PendingCount)},
			ecsServiceStabilityCell(&services[i]),
		}
	}
	// Only the name has no natural width, so it is the one column that flexes and the counts and badge keep their full text.
	table, _ := utils.RenderTableFit(rows, width, []int{1, 0, 0, 0})

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
			StatusCellFit(t.Status, StatusStyleIcon),
			{Text: t.ID, Color: color.Faint},
			{Text: ecsTaskImageText(t)},
		}
	}
	// The image is the column with no natural width, so it carries the weight. Being last, it renders identically to a content-sized column at every width measured, and the weight is what keeps that true if a column is ever added after it.
	table, _ := utils.RenderTableFit(rows, width, []int{0, 0, 1})

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

func GetECSTaskDisplayCells(t *aws.ECSTask) []utils.Cell {
	return []utils.Cell{
		StatusCellFit(t.Status, StatusStyleIcon),
		{Text: t.ID, Color: color.FgMagenta},
		{Text: t.LaunchType},
	}
}
