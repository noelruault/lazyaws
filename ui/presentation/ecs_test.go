package presentation

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

func cellTexts(cells []utils.Cell) []string {
	out := make([]string, len(cells))
	for i, cell := range cells {
		out[i] = cell.Text
	}
	return out
}

func wantCells(t *testing.T, got []utils.Cell, want []utils.Cell) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got %d cells %q, want %d", len(got), cellTexts(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cell %d = %+v, want %+v (row %q)", i, got[i], want[i], cellTexts(got))
		}
	}
}

func TestGetECSClusterDisplayCells(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cluster *aws.ECSCluster
		want    []utils.Cell
	}{
		{
			"pending tasks read as a rollout in progress",
			&aws.ECSCluster{Name: "prod", Status: "ACTIVE", RunningTasksCount: 3, PendingTasksCount: 1, ActiveServicesCount: 2},
			[]utils.Cell{
				{Text: "▶", Color: color.FgGreen},
				{Text: "prod"},
				{Text: "2 services", Color: color.FgYellow},
				{Text: "3 running / 1 pending"},
				{Text: "● deploying", Color: color.FgYellow},
			},
		},
		{
			"everything up and nothing pending is healthy",
			&aws.ECSCluster{Name: "prod", Status: "ACTIVE", RunningTasksCount: 3, ActiveServicesCount: 2},
			[]utils.Cell{
				{Text: "▶", Color: color.FgGreen},
				{Text: "prod"},
				{Text: "2 services", Color: color.FgYellow},
				{Text: "3 running / 0 pending"},
				{Text: "● healthy", Color: color.FgGreen},
			},
		},
		{
			"an empty active cluster is still healthy",
			&aws.ECSCluster{Name: "empty", Status: "ACTIVE"},
			[]utils.Cell{
				{Text: "▶", Color: color.FgGreen},
				{Text: "empty"},
				{Text: "0 services", Color: color.FgYellow},
				{Text: "0 running / 0 pending"},
				{Text: "● healthy", Color: color.FgGreen},
			},
		},
		{
			"a non-active cluster shows its own status word",
			&aws.ECSCluster{Name: "old", Status: "INACTIVE", ActiveServicesCount: 1},
			[]utils.Cell{
				{Text: "⨯", Color: color.FgRed},
				{Text: "old"},
				{Text: "1 services", Color: color.FgYellow},
				{Text: "0 running / 0 pending"},
				{Text: "● INACTIVE", Color: color.FgRed},
			},
		},
		{
			"tasks draining out of a dying cluster are not a deployment",
			&aws.ECSCluster{Name: "old", Status: "DEPROVISIONING", PendingTasksCount: 2},
			[]utils.Cell{
				{Text: "?", Color: color.FgWhite},
				{Text: "old"},
				{Text: "0 services", Color: color.FgYellow},
				{Text: "0 running / 2 pending"},
				{Text: "● DEPROVISIONING", Color: color.FgRed},
			},
		},
		{
			"a status AWS did not return is named, not left as a bare bullet",
			&aws.ECSCluster{Name: "mystery"},
			[]utils.Cell{
				{Text: "?", Color: color.FgWhite},
				{Text: "mystery"},
				{Text: "0 services", Color: color.FgYellow},
				{Text: "0 running / 0 pending"},
				{Text: "● unknown", Color: color.FgRed},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantCells(t, GetECSClusterDisplayCells(tt.cluster), tt.want)
		})
	}
}

func TestGetECSServiceDisplayCells(t *testing.T) {
	s := &aws.ECSService{Name: "web", Status: "ACTIVE", LaunchType: "FARGATE", DesiredCount: 2, RunningCount: 2}

	wantCells(t, GetECSServiceDisplayCells(s), []utils.Cell{
		{Text: "▶", Color: color.FgGreen},
		{Text: "web"},
		{Text: "FARGATE", Color: color.FgYellow},
		{Text: "2/2"},
	})
}

func TestGetECSTaskDisplayCells(t *testing.T) {
	tsk := &aws.ECSTask{ID: "abc123", Status: "RUNNING", LaunchType: "FARGATE"}

	wantCells(t, GetECSTaskDisplayCells(tsk), []utils.Cell{
		{Text: "▶", Color: color.FgGreen},
		{Text: "abc123", Color: color.FgMagenta},
		{Text: "FARGATE"},
	})
}

// The cluster inspector still lays its services table out with RenderTable, so the string form has to keep carrying the colour the cells describe.
func TestGetECSServiceDisplayStringsColoursTheCells(t *testing.T) {
	forceColor(t)
	s := &aws.ECSService{Name: "web", Status: "ACTIVE", LaunchType: "FARGATE", DesiredCount: 2, RunningCount: 2}

	got := GetECSServiceDisplayStrings(s)

	want := []string{
		utils.ColoredString("▶", color.FgGreen),
		"web",
		utils.ColoredString("FARGATE", color.FgYellow),
		"2/2",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d cells %q, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cell %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// Every weight table must match the row it lays out, or RenderTableFit rejects the whole table at render time.
func TestECSWeightsMatchTheirRowWidths(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cells   int
		weights []int
	}{
		{"cluster", len(GetECSClusterDisplayCells(&aws.ECSCluster{})), ECSClusterWeights()},
		{"service", len(GetECSServiceDisplayCells(&aws.ECSService{})), ECSServiceWeights()},
		{"task", len(GetECSTaskDisplayCells(&aws.ECSTask{})), ECSTaskWeights()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.weights) != tt.cells {
				t.Errorf("%d weights for %d cells", len(tt.weights), tt.cells)
			}
		})
	}
}

// A side panel is routinely narrower than the five columns this row wants, and the name is the one thing that makes the row identifiable.
// Degrading right to left is safe here: the leftmost cell is the status icon, so dropping the badge does not drop the status.
func TestClusterRowKeepsTheNameInANarrowPanel(t *testing.T) {
	c := &aws.ECSCluster{Name: "production-eu-west-1-primary", Status: "ACTIVE", RunningTasksCount: 12, PendingTasksCount: 1, ActiveServicesCount: 9}

	for _, width := range []int{30, 40, 50, 60, 80} {
		rendered, err := utils.RenderTableFit([][]utils.Cell{GetECSClusterDisplayCells(c)}, width, ECSClusterWeights())
		if err != nil {
			t.Fatalf("width %d: %v", width, err)
		}
		if got := runewidth.StringWidth(rendered); got > width {
			t.Errorf("width %d: row is %d cells wide: %q", width, got, rendered)
		}
		if !strings.HasPrefix(rendered, "▶ product") {
			t.Errorf("width %d: row = %q, want the cluster name after the status icon", width, rendered)
		}
	}
}

func TestECSImageSummary(t *testing.T) {
	tests := []struct {
		name  string
		image aws.ECSServiceImage
		want  string
	}{
		{"no sidecars", aws.ECSServiceImage{Image: "app-auth:v1.2.0-develop.0"}, "app-auth:v1.2.0-develop.0"},
		{"one sidecar reads singular", aws.ECSServiceImage{Image: "app-auth:v1.2.0-develop.0", Sidecars: 1}, "app-auth:v1.2.0-develop.0 (+1 sidecar)"},
		{"several sidecars read plural", aws.ECSServiceImage{Image: "web:1", Sidecars: 3}, "web:1 (+3 sidecars)"},
		{"nothing resolved is stated, not blank", aws.ECSServiceImage{}, "unavailable"},
		// A count that somehow went negative is a bug in the caller, not a reason to render "(+-1 sidecar)".
		{"negative count is dropped", aws.ECSServiceImage{Image: "web:1", Sidecars: -1}, "web:1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ECSImageSummary(tt.image); got != tt.want {
				t.Errorf("ECSImageSummary(%+v) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}

func TestECSImageLabel(t *testing.T) {
	if got := ECSImageLabel(aws.ECSServiceImage{Image: "web:1"}); got != "Running image" {
		t.Errorf("ECSImageLabel() = %q, want %q for an image read off a running container", got, "Running image")
	}
	if got := ECSImageLabel(aws.ECSServiceImage{Image: "web:1", Desired: true}); got != "Desired image" {
		t.Errorf("ECSImageLabel() = %q, want %q so a task-definition image is never read as live", got, "Desired image")
	}
}

// clusterFixture is a healthy three-service cluster the overview tests vary one thing at a time from.
func clusterFixture() (*aws.ECSCluster, *aws.ECSClusterOverview) {
	at := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	point := func(v float64) aws.MetricPoint { return aws.MetricPoint{Value: v, At: at, OK: true} }

	cluster := &aws.ECSCluster{
		Name:                     "batch-cluster",
		Arn:                      "arn:aws:ecs:eu-west-1:123456789012:cluster/batch-cluster",
		Status:                   "ACTIVE",
		RunningTasksCount:        12,
		ActiveServicesCount:      3,
		RegisteredContainerCount: 2,
		ContainerInsights:        "enabled",
		ExecuteCommandLogging:    "DEFAULT",
		Region:                   "eu-west-1",
		ConsoleURL:               "https://eu-west-1.console.aws.amazon.com/ecs/v2/clusters/batch-cluster",
	}
	overview := &aws.ECSClusterOverview{
		Errs: map[string]error{},
		Tags: map[string]string{"Environment": "staging"},
		Services: []aws.ECSService{
			{Name: "kicker-web", RunningCount: 3, DesiredCount: 3, LaunchType: "FARGATE",
				Deployments: []aws.ECSDeployment{{Status: "PRIMARY", RolloutState: "COMPLETED"}}},
		},
		Tasks: []aws.ECSTask{
			{ID: "a1b2c3d4e5f6", Status: "RUNNING", Containers: []aws.ECSContainer{
				{Name: "app", ImageURI: "123456789012.dkr.ecr.eu-west-1.amazonaws.com/kicker-web:v1.42.0", Essential: true},
				{Name: "fluentbit", ImageURI: "public.ecr.aws/aws-observability/aws-for-fluent-bit:stable"},
			}},
		},
		Metrics: &aws.ECSClusterMetrics{
			CPUUsed: point(268), CPUReserved: point(1024),
			MemUsed: point(1433), MemReserved: point(2048),
		},
	}

	return cluster, overview
}

// The overview is rendered below minTwoColWidth so each block is laid out whole; above it Columns interleaves the two blocks line by line and cuts each to its own column.
const overviewTestWidth = 100

// lineContaining returns the whole rendered line holding needle, so an assertion can pin what the line ENDS on.
// A Contains check against the pane cannot: a line that grew an extra entry still contains the shorter text it was asked about.
func lineContaining(pane, needle string) string {
	for _, line := range strings.Split(pane, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}

	return ""
}

func TestClusterOverviewRendersEverySection(t *testing.T) {
	forceColor(t)
	cluster, overview := clusterFixture()

	got := utils.Decolorise(FormatECSClusterOverview(cluster, overview, overviewTestWidth))

	for _, want := range []string{
		"batch-cluster",
		"eu-west-1",
		// Every count lives in exactly one place after the dedup pass: the cards carry services, running and pending, and the Health block carries what no card does.
		"1 / 1",
		"12 running",
		"Deployments:",
		"arn:aws:ecs:eu-west-1:123456789012:cluster/batch-cluster",
		"Container Insights: enabled",
		"DEFAULT (task awslogs driver)",
		"kicker-web",
		// The service table spells its columns out; the row under them is asserted by the narrow-width test.
		"Desired", "Running", "Pending",
		"● steady",
		"a1b2c3d4e5f6",
		// The image is the hard requirement this pane exists for, with the registry host dropped and the sidecar counted rather than listed.
		"kicker-web:v1.42.0 (+1 sidecar)",
		// What the old Config and Tags tabs held and the Overview absorbed: the console URL, the tag list and the container-instance count.
		"https://eu-west-1.console.aws.amazon.com/ecs/v2/clusters/batch-cluster",
		"Environment: staging",
		"Container instances: 2",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview does not contain %q\n%s", want, got)
		}
	}
}

// Both gauges have to carry the absolute readings behind them: a percentage alone cannot tell a small busy cluster from a large idle one.
func TestClusterOverviewMetricsGaugesCarryTheirReadings(t *testing.T) {
	forceColor(t)
	cluster, overview := clusterFixture()

	got := utils.Decolorise(FormatECSClusterOverview(cluster, overview, overviewTestWidth))

	for _, want := range []string{"26.2%", "268 / 1024 units", "70.0%", "1433 / 2048 MiB", "█", "░"} {
		if !strings.Contains(got, want) {
			t.Errorf("metrics section does not contain %q\n%s", want, got)
		}
	}
}

// A cluster with Insights switched off is one setting away from having numbers; a cluster CloudWatch failed to answer for is not, and rendering both as "no data" hides the difference.
func TestClusterOverviewSeparatesInsightsOffFromAFailedFetch(t *testing.T) {
	forceColor(t)
	cluster, overview := clusterFixture()
	overview.Metrics, overview.InsightsOff = nil, true

	off := utils.Decolorise(FormatECSClusterOverview(cluster, overview, overviewTestWidth))
	if !strings.Contains(off, "Container Insights off, no metrics") {
		t.Errorf("Insights off must say so rather than reading as missing data\n%s", off)
	}
	if strings.Contains(off, "unavailable:") {
		t.Errorf("Insights off is not a failed fetch and must not be reported as one\n%s", off)
	}

	overview.InsightsOff = false
	overview.Errs[aws.SectionMetrics] = errors.New("AccessDenied: cloudwatch:GetMetricData")
	failed := utils.Decolorise(FormatECSClusterOverview(cluster, overview, overviewTestWidth))
	if !strings.Contains(failed, "unavailable: AccessDenied: cloudwatch:GetMetricData") {
		t.Errorf("a failed metrics fetch must keep its reason on screen: the pane retries on a ticker, so a throttle and a denial otherwise look identical\n%s", failed)
	}
}

// A reservation CloudWatch never answered for and a cluster reserving nothing both compute to 0.0%, and a gauge sitting at zero is the more believable of the two lies.
func TestClusterOverviewGaugeReportsNoDataRatherThanZero(t *testing.T) {
	forceColor(t)
	cluster, overview := clusterFixture()
	overview.Metrics = &aws.ECSClusterMetrics{CPUUsed: aws.MetricPoint{Value: 268, At: time.Now(), OK: true}}

	got := utils.Decolorise(FormatECSClusterOverview(cluster, overview, overviewTestWidth))

	if !strings.Contains(got, "CPU:    no data") {
		t.Errorf("an absent reservation must render no data, never a 0.0%% bar\n%s", got)
	}
	if strings.Contains(got, "0.0%") {
		t.Errorf("no gauge may be drawn from an absent reading\n%s", got)
	}
}

// The ticket's own case: a cluster with no capacity providers still places tasks, and "none" on its own reads as broken rather than as configured differently.
func TestClusterOverviewCapacityFallsBackToTheServiceLaunchType(t *testing.T) {
	forceColor(t)
	cluster, overview := clusterFixture()
	// A second FARGATE service, so the line can only read once if it deduplicates; and EC2 listed after it, so it can only read in order if it sorts.
	overview.Services = append(overview.Services,
		aws.ECSService{Name: "kicker-batch", LaunchType: "FARGATE"},
		aws.ECSService{Name: "kicker-cron", LaunchType: "EC2"},
	)

	got := utils.Decolorise(FormatECSClusterOverview(cluster, overview, overviewTestWidth))

	// Asserted as the WHOLE line, not as a substring: "EC2, FARGATE, FARGATE" contains "EC2, FARGATE" too, so a Contains check cannot see a lost deduplication.
	if line := lineContaining(got, "services launch on"); line != "  none, services launch on EC2, FARGATE" {
		t.Errorf("capacity line = %q, want each launch type named once in a stable order\n%s", line, got)
	}

	// With providers configured the strategy is what places a task, and the fallback must not be shown alongside it.
	cluster.DefaultCapacityProviderStrat = []aws.ECSCapacityProviderStrategy{{CapacityProvider: "FARGATE_SPOT", Base: 2, Weight: 4}}
	withProviders := utils.Decolorise(FormatECSClusterOverview(cluster, overview, overviewTestWidth))
	if !strings.Contains(withProviders, "FARGATE_SPOT (base 2, weight 4)") {
		t.Errorf("capacity must show the default strategy's base and weight\n%s", withProviders)
	}
	if strings.Contains(withProviders, "services launch on") {
		t.Errorf("the launch-type fallback is for a cluster with no providers, not an extra line beside them\n%s", withProviders)
	}

	// Providers attached with no default strategy is its own state: a service must then name one itself.
	cluster.DefaultCapacityProviderStrat = nil
	cluster.CapacityProviders = []string{"FARGATE", "FARGATE_SPOT"}
	noStrategy := utils.Decolorise(FormatECSClusterOverview(cluster, overview, overviewTestWidth))
	if !strings.Contains(noStrategy, "FARGATE, FARGATE_SPOT") || !strings.Contains(noStrategy, "no default strategy") {
		t.Errorf("providers without a default strategy must say so\n%s", noStrategy)
	}
}

// The ticket's own case: any rollout that is not COMPLETED is deploying, and failed tasks outrank it because ECS retries them, so a stuck rollout stays IN_PROGRESS forever.
func TestClusterOverviewServiceStability(t *testing.T) {
	cases := []struct {
		name    string
		service aws.ECSService
		want    utils.Cell
	}{
		{
			name: "rollout in progress",
			service: aws.ECSService{RunningCount: 3, DesiredCount: 3,
				Deployments: []aws.ECSDeployment{{RolloutState: "IN_PROGRESS"}}},
			want: utils.Cell{Text: "● deploying", Color: color.FgYellow},
		},
		{
			name: "failed tasks outrank a rollout still in progress",
			service: aws.ECSService{RunningCount: 1, DesiredCount: 3,
				Deployments: []aws.ECSDeployment{{RolloutState: "IN_PROGRESS", FailedTasks: 4}}},
			want: utils.Cell{Text: "● 4 failed", Color: color.FgRed},
		},
		{
			name: "a failed rollout that lost no tasks is still red",
			service: aws.ECSService{RunningCount: 3, DesiredCount: 3,
				Deployments: []aws.ECSDeployment{{RolloutState: "FAILED"}}},
			want: utils.Cell{Text: "● failed", Color: color.FgRed},
		},
		{
			// A CODE_DEPLOY or EXTERNAL controller reports no rolloutState at all, and reading absent as "not COMPLETED" leaves every blue/green service permanently amber.
			name: "an absent rollout state is settled, not deploying",
			service: aws.ECSService{RunningCount: 3, DesiredCount: 3,
				Deployments: []aws.ECSDeployment{{Status: "PRIMARY"}}},
			want: utils.Cell{Text: "● steady", Color: color.FgGreen},
		},
		{
			name: "short of its desired count with the rollout done",
			service: aws.ECSService{RunningCount: 1, DesiredCount: 3,
				Deployments: []aws.ECSDeployment{{RolloutState: "COMPLETED"}}},
			want: utils.Cell{Text: "● scaling", Color: color.FgYellow},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ecsServiceStabilityCell(&tc.service); got != tc.want {
				t.Errorf("ecsServiceStabilityCell() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// The reason is the only thing on the pane that says WHY a rollout went wrong, and it is a sentence: in a table cell it would be cut to its least specific few words.
func TestClusterOverviewShowsTheRolloutReasonOnItsOwnLine(t *testing.T) {
	forceColor(t)
	cluster, overview := clusterFixture()
	reason := "ECS deployment circuit breaker: task failed to start."
	overview.Services = []aws.ECSService{{
		Name: "kicker-worker", RunningCount: 0, DesiredCount: 2, LaunchType: "FARGATE",
		Deployments: []aws.ECSDeployment{{Status: "PRIMARY", RolloutState: "FAILED", FailedTasks: 4, RolloutStateReason: reason}},
	}}

	got := utils.Decolorise(FormatECSClusterOverview(cluster, overview, overviewTestWidth))

	if !strings.Contains(got, "kicker-worker: "+reason) {
		t.Errorf("a failed rollout must name the service and its reason\n%s", got)
	}
	// The steady count lives in the Services card now; a service that lost every task is not steady.
	if !strings.Contains(got, "0 / 1") {
		t.Errorf("a service that lost every task is not steady\n%s", got)
	}

	// A healthy service has a reason field ECS fills with its success text, and putting that on the pane buries the failures.
	overview.Services[0].Deployments = []aws.ECSDeployment{{Status: "PRIMARY", RolloutState: "COMPLETED", RolloutStateReason: "ECS deployment ecs-svc/123 completed."}}
	healthy := utils.Decolorise(FormatECSClusterOverview(cluster, overview, overviewTestWidth))
	if strings.Contains(healthy, "completed.") {
		t.Errorf("a completed rollout's reason is noise and must not be given a line\n%s", healthy)
	}
}

// The ticket's own case, and the one every empty state has to survive: the pane must still identify the cluster rather than rendering blank blocks.
func TestClusterOverviewEmptyStates(t *testing.T) {
	forceColor(t)
	cluster := &aws.ECSCluster{Name: "empty-cluster", Arn: "arn:aws:ecs:eu-west-1:123456789012:cluster/empty-cluster", Status: "ACTIVE", Region: "eu-west-1"}
	overview := &aws.ECSClusterOverview{Errs: map[string]error{}, InsightsOff: true}

	got := utils.Decolorise(FormatECSClusterOverview(cluster, overview, overviewTestWidth))

	for _, want := range []string{
		"empty-cluster",
		"no services",
		"no running tasks",
		"none, no services to place",
		// Empty is not disabled: it means the describe call did not ask, and the two call for different actions.
		"Container Insights: unknown",
		"Execute command:    not configured",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("empty overview does not contain %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "0/0 services steady") {
		t.Errorf("a cluster with no services has nothing to be steady\n%s", got)
	}
}

// Each fetch fails on its own, and the header is built from the LIST ROW so a cluster whose every section failed is still identified.
func TestClusterOverviewSectionsFailIndependently(t *testing.T) {
	forceColor(t)
	cluster, overview := clusterFixture()
	overview.Errs[aws.SectionServices] = errors.New("AccessDenied: ecs:ListServices")

	got := utils.Decolorise(FormatECSClusterOverview(cluster, overview, overviewTestWidth))

	if !strings.Contains(got, "unavailable: AccessDenied: ecs:ListServices") {
		t.Errorf("a failed services fetch must state its reason\n%s", got)
	}
	// The Services card cannot count steady services without the list, and falls back to the count the cluster itself reported.
	if card := clusterStatCards(cluster, overview)[0].Value.Text; card != "3" {
		t.Errorf("with the service list unavailable the Services card reads %q, want the cluster's own count %q", card, "3")
	}
	if !strings.Contains(got, "none, services unavailable") {
		t.Errorf("capacity cannot name launch types it could not read, and must say so rather than assuming Fargate\n%s", got)
	}
	// The sections that did not depend on that fetch still render.
	if !strings.Contains(got, "a1b2c3d4e5f6") || !strings.Contains(got, "26.2%") {
		t.Errorf("one failed section must not blank the sections that succeeded\n%s", got)
	}
}

// The pane re-renders every couple of seconds and neither ListServices nor DescribeTasks promises an order, so rows that followed the response would reshuffle under the cursor between refreshes.
func TestClusterOverviewRowsAreSortedRatherThanInResponseOrder(t *testing.T) {
	forceColor(t)
	cluster, overview := clusterFixture()
	overview.Services = []aws.ECSService{
		{Name: "zebra", RunningCount: 1, DesiredCount: 1},
		{Name: "alpha", RunningCount: 1, DesiredCount: 1},
	}
	overview.Tasks = []aws.ECSTask{
		{ID: "ffff2222", Status: "RUNNING", Containers: []aws.ECSContainer{{Name: "app", ImageURI: "kicker:v1", Essential: true}}},
		{ID: "0000aaaa", Status: "RUNNING", Containers: []aws.ECSContainer{{Name: "app", ImageURI: "kicker:v1", Essential: true}}},
	}

	got := utils.Decolorise(FormatECSClusterOverview(cluster, overview, overviewTestWidth))

	if strings.Index(got, "alpha") > strings.Index(got, "zebra") {
		t.Errorf("services must render in name order, not the order ListServices answered in\n%s", got)
	}
	if strings.Index(got, "0000aaaa") > strings.Index(got, "ffff2222") {
		t.Errorf("tasks must render in id order, not the order DescribeTasks answered in\n%s", got)
	}
}

// A busy cluster runs hundreds of tasks against a pane that shows a screenful, and a silent cap reads as the whole list.
func TestClusterOverviewCapsTheTasksTable(t *testing.T) {
	forceColor(t)
	cluster, overview := clusterFixture()
	overview.Tasks = make([]aws.ECSTask, ecsOverviewTasksShown+3)
	for i := range overview.Tasks {
		overview.Tasks[i] = aws.ECSTask{
			ID:         fmt.Sprintf("task%02d", i),
			Status:     "RUNNING",
			Containers: []aws.ECSContainer{{Name: "app", ImageURI: "kicker:v1", Essential: true}},
		}
	}

	got := utils.Decolorise(FormatECSClusterOverview(cluster, overview, overviewTestWidth))

	if !strings.Contains(got, "(3 more)") {
		t.Errorf("the hidden task count must be stated\n%s", got)
	}
	if strings.Contains(got, "task16") {
		t.Errorf("the table must stop at the cap\n%s", got)
	}
	// Sorted, because the pane re-renders on a ticker and DescribeTasks promises no order.
	if !strings.Contains(got, "task00") || !strings.Contains(got, "task14") {
		t.Errorf("the first %d tasks in id order must be the ones shown\n%s", ecsOverviewTasksShown, got)
	}
}

// A task whose containers could not be read has no image to name, and a blank column in the one place this pane exists for reads as a rendering bug.
func TestClusterOverviewTaskWithoutContainersSaysUnavailable(t *testing.T) {
	forceColor(t)
	cluster, overview := clusterFixture()
	overview.Tasks = []aws.ECSTask{{ID: "a1b2c3d4e5f6", Status: "PENDING"}}

	got := utils.Decolorise(FormatECSClusterOverview(cluster, overview, overviewTestWidth))

	row := lineContaining(got, "a1b2c3d4e5f6")
	if !strings.Contains(row, "unavailable") {
		t.Errorf("a task with no readable containers must say so in the image column\n%s", got)
	}
}

// Measured, not assumed: content-sizing the service name renders tighter at every width but hands a long name the whole row, which deletes the counts and the stability badge outright.
// The flexible name is what keeps them, so this pins the degradation rather than the happy path.
func TestClusterOverviewNarrowServiceRowKeepsItsStabilityBadge(t *testing.T) {
	forceColor(t)
	cluster, overview := clusterFixture()
	overview.Services = []aws.ECSService{{
		Name: "a-very-long-service-name-nobody-should-have-but-someone-does", RunningCount: 1, DesiredCount: 3, PendingCount: 2,
		Deployments: []aws.ECSDeployment{{RolloutState: "IN_PROGRESS"}},
	}}

	// Either side of minTwoColWidth, so both the stacked and the two-column layouts are covered.
	// The counts read left to right as Desired, Running, Pending, so their order in the row is part of what is pinned.
	countsThenBadge := regexp.MustCompile(`3\s+1\s+2\s+● deploying`)
	for _, width := range []int{80, 110, 120, 160} {
		got := utils.Decolorise(FormatECSClusterOverview(cluster, overview, width))

		row := lineContaining(got, "a-very-long")
		// A prefix short enough to survive the narrowest column here: the point is that the name is still identifiable, not how many of its cells were paid for.
		if row == "" {
			t.Errorf("at width %d the service name was cut away entirely\n%s", width, got)
			continue
		}
		if !countsThenBadge.MatchString(row) {
			t.Errorf("at width %d the service row lost its counts or its badge to a long name: %q", width, row)
		}
	}
}

// Wrapping is off on an overview, so a line over its budget runs off the pane rather than folding.
func TestClusterOverviewNeverExceedsTheWidth(t *testing.T) {
	forceColor(t)
	cluster, overview := clusterFixture()
	overview.Services = append(overview.Services, aws.ECSService{
		Name: "a-very-long-service-name-nobody-should-have-but-someone-does", RunningCount: 1, DesiredCount: 3,
		Deployments: []aws.ECSDeployment{{RolloutState: "FAILED", FailedTasks: 2, RolloutStateReason: "ECS deployment circuit breaker: task failed to start and the reason ECS gives runs well past any column."}},
	})

	for width := 40; width <= 220; width++ {
		for _, line := range strings.Split(FormatECSClusterOverview(cluster, overview, width), "\n") {
			if got := runewidth.StringWidth(utils.Decolorise(line)); got > width {
				t.Fatalf("at width %d a line is %d cells wide: %q", width, got, utils.Decolorise(line))
			}
		}
	}
}
