package presentation

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// serviceFixture is a healthy service holding its desired count, which the overview tests vary one thing at a time from.
// Its events are deliberately OLDEST FIRST: a fixture already in the order the pane wants cannot tell a sort from no sort at all.
func serviceFixture() (*aws.ECSService, *aws.ECSServiceOverview, time.Time) {
	now := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	created := now.Add(-6 * time.Hour)
	published := now.Add(-2 * time.Minute)
	oldest, newest := now.Add(-3*time.Hour), now.Add(-20*time.Minute)
	point := func(v float64) aws.MetricPoint { return aws.MetricPoint{Value: v, At: published, OK: true} }

	service := &aws.ECSService{
		Name:                   "app-auth",
		Arn:                    "arn:aws:ecs:eu-west-1:123456789012:service/app-cluster/app-auth",
		Status:                 "ACTIVE",
		Cluster:                "app-cluster",
		Region:                 "eu-west-1",
		LaunchType:             "FARGATE",
		TaskDefinition:         "arn:aws:ecs:eu-west-1:123456789012:task-definition/app-auth:41",
		DesiredCount:           3,
		RunningCount:           3,
		DeploymentController:   "ECS",
		CircuitBreakerEnabled:  true,
		CircuitBreakerRollback: true,
		Deployments: []aws.ECSDeployment{{
			Status:         aws.ECSDeploymentPrimary,
			RolloutState:   aws.ECSRolloutCompleted,
			TaskDefinition: "arn:aws:ecs:eu-west-1:123456789012:task-definition/app-auth:42",
			Created:        &created,
			Desired:        3,
			Running:        3,
		}},
		Network: &aws.ECSAwsVpcConfig{
			Subnets:        []string{"subnet-0a1b2c3d4e5f60718", "subnet-1a2b3c4d5e6f70819"},
			SecurityGroups: []string{"sg-0a1b2c3d4e5f60718"},
			AssignPublicIP: "DISABLED",
		},
		Events: []aws.ECSEvent{
			{Message: "(service app-auth) has started 1 tasks.", When: &oldest},
			{Message: "(service app-auth) has reached a steady state.", When: &newest},
		},
	}

	overview := &aws.ECSServiceOverview{
		Errs:    map[string]error{},
		Metrics: &aws.ECSServiceMetrics{CPUUtilization: point(37.5), MemoryUtilization: point(64.2)},
		Image:   aws.ECSServiceImage{Image: "app-auth:v1.2.0", Sidecars: 1},
		Scaling: &aws.ECSServiceAutoScaling{
			MinCapacity: 1,
			MaxCapacity: 5,
			Policies: []aws.ECSScalingPolicy{
				{Name: "cpu-target", Type: "TargetTrackingScaling", TargetMetric: "ECSServiceAverageCPUUtilization", TargetValue: 60, ScaleInCooldownSecs: 60, ScaleOutCooldownSecs: 30},
				{Name: "step-out", Type: "StepScaling", StepAdjustments: 2, ScaleOutCooldownSecs: 120},
			},
		},
	}

	return service, overview, now
}

func TestServiceOverviewRendersEverySection(t *testing.T) {
	forceColor(t)
	service, overview, now := serviceFixture()

	got := utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))

	for _, want := range []string{
		"app-auth",
		"app-cluster",
		"FARGATE",
		// The ticket's header contract: the badge plus all three counts.
		"● steady",
		"3 desired / 3 running / 0 pending",
		"Controller:      ECS",
		"● COMPLETED",
		"Started:         6h ago",
		"Circuit breaker: enabled, rolls back",
		// The PRIMARY deployment's revision, not the service's own: mid-rollout they differ and the deployment is what is being brought up.
		"Task definition: app-auth:42",
		"Running image:   app-auth:v1.2.0 (+1 sidecar)",
		"Public IP: DISABLED",
		"Subnets (2):",
		"subnet-0a1b2c3d4e5f60718",
		"Security groups (1):",
		"sg-0a1b2c3d4e5f60718",
		"37.5%",
		"64.2%",
		"1-min avg @ 14:58Z",
		// What the old Scaling tab held and the Overview absorbed: bounds, then each policy with its target.
		"Capacity: 1 - 5 tasks",
		"cpu-target TargetTrackingScaling",
		"target 60.0 (ECSServiceAverageCPUUtilization), cooldown in 60s / out 30s",
		"step-out StepScaling",
		"2 step adjustment(s), cooldown 120s",
		"Recent events",
		"has reached a steady state.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("service overview does not contain %q\n%s", want, got)
		}
	}
}

// A service with no Application Auto Scaling registered is an answer, not a failure, and a failed scaling read must say so instead of rendering as that answer.
func TestServiceOverviewScalingStates(t *testing.T) {
	forceColor(t)
	service, overview, now := serviceFixture()

	overview.Scaling = nil
	got := utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))
	if !strings.Contains(got, "not registered") {
		t.Errorf("an unregistered service should say so\n%s", got)
	}

	overview.Errs[aws.SectionScaling] = errors.New("ThrottlingException")
	got = utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))
	if !strings.Contains(got, "unavailable: ThrottlingException") {
		t.Errorf("a failed scaling read should render unavailable\n%s", got)
	}
	if strings.Contains(got, "not registered") {
		t.Errorf("a failed scaling read must not render as the unregistered answer\n%s", got)
	}
}

// The ticket's own case: a service short of what it was asked to run is the one an operator opens this pane for, and every count that says so has to be on it.
func TestServiceOverviewShowsDesiredApartFromRunning(t *testing.T) {
	forceColor(t)
	service, overview, now := serviceFixture()
	service.DesiredCount, service.RunningCount, service.PendingCount = 5, 2, 1

	got := utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))

	if !strings.Contains(got, "5 desired / 2 running / 1 pending") {
		t.Errorf("the header must carry all three counts\n%s", got)
	}
	// A settled rollout that is short of its desired count is scaling, not steady: the rollout finished and the service still does not have what it asked for.
	if !strings.Contains(got, "● scaling") {
		t.Errorf("a service below its desired count must not read as steady\n%s", got)
	}
}

// A rollout is the one thing on this pane that says what the service is DOING, and its reason is the only thing that says why it went wrong.
func TestServiceOverviewRolloutStates(t *testing.T) {
	forceColor(t)

	for _, tc := range []struct {
		name       string
		deployment aws.ECSDeployment
		want       []string
		absent     string
	}{
		{
			name:       "in progress",
			deployment: aws.ECSDeployment{Status: aws.ECSDeploymentPrimary, RolloutState: "IN_PROGRESS"},
			want:       []string{"● IN_PROGRESS"},
		},
		{
			name:       "failed with a reason and lost tasks",
			deployment: aws.ECSDeployment{Status: aws.ECSDeploymentPrimary, RolloutState: aws.ECSRolloutFailed, FailedTasks: 2, RolloutStateReason: "ECS deployment circuit breaker: task failed to start."},
			want:       []string{"● FAILED", "2 failed", "ECS deployment circuit breaker: task failed to start."},
		},
		{
			// ECS omits rolloutState entirely for a CODE_DEPLOY or EXTERNAL controller, and reading that as "not COMPLETED" would leave every blue/green service permanently alarming.
			name:       "a controller that reports no rollout",
			deployment: aws.ECSDeployment{Status: aws.ECSDeploymentPrimary},
			want:       []string{"Rollout:         not reported"},
			absent:     "FAILED",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, overview, now := serviceFixture()
			service.Deployments = []aws.ECSDeployment{tc.deployment}

			got := utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))

			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("rollout section does not contain %q\n%s", want, got)
				}
			}
			if tc.absent != "" && strings.Contains(got, tc.absent) {
				t.Errorf("rollout section must not contain %q\n%s", tc.absent, got)
			}
		})
	}
}

// A service between deployments has none at all, which must render as a rollout nobody reported rather than panic or read as a failure.
func TestServiceOverviewWithoutDeployments(t *testing.T) {
	forceColor(t)
	service, overview, now := serviceFixture()
	service.Deployments = nil

	got := utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))

	if !strings.Contains(got, "Rollout:         not reported") {
		t.Errorf("a service with no deployment must say the rollout was not reported\n%s", got)
	}
	if !strings.Contains(got, "Started:         unknown") {
		t.Errorf("a deployment with no creation time must say unknown rather than claim an age\n%s", got)
	}
	// With no deployment naming one, the service's own task definition is the answer.
	if !strings.Contains(got, "Task definition: app-auth:41") {
		t.Errorf("the task definition must fall back to the service's own revision\n%s", got)
	}
}

// The breaker stops a bad deployment whether or not it rolls back, and only one of those restores the revision that was working.
func TestServiceOverviewCircuitBreakerStates(t *testing.T) {
	forceColor(t)

	for _, tc := range []struct {
		enabled, rollback bool
		want              string
	}{
		{false, false, "Circuit breaker: disabled"},
		{false, true, "Circuit breaker: disabled"},
		{true, false, "Circuit breaker: enabled, no rollback"},
		{true, true, "Circuit breaker: enabled, rolls back"},
	} {
		service, overview, now := serviceFixture()
		service.CircuitBreakerEnabled, service.CircuitBreakerRollback = tc.enabled, tc.rollback

		got := utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))

		if !strings.Contains(got, tc.want) {
			t.Errorf("enabled=%v rollback=%v does not render %q\n%s", tc.enabled, tc.rollback, tc.want, got)
		}
	}
}

// A service with nothing running still has an image it intends to run, and labelling that as running would be a lie the pane cannot walk back.
func TestServiceOverviewLabelsADesiredImage(t *testing.T) {
	forceColor(t)
	service, overview, now := serviceFixture()
	service.RunningCount, service.DesiredCount = 0, 0
	overview.Image = aws.ECSServiceImage{Image: "app-auth:v1.2.0", Desired: true}

	got := utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))

	if !strings.Contains(got, "Desired image:   app-auth:v1.2.0") {
		t.Errorf("an image read off a task definition must be labelled desired\n%s", got)
	}
	if strings.Contains(got, "Running image") {
		t.Errorf("nothing is running, so no line may claim a running image\n%s", got)
	}
}

// A service whose task definition uses bridge or host networking carries no configuration at all, which is a different answer from one whose subnets could not be read.
func TestServiceOverviewWithoutAwsvpcNetworking(t *testing.T) {
	forceColor(t)
	service, overview, now := serviceFixture()
	service.Network = nil

	got := utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))

	if !strings.Contains(got, "no awsvpc configuration") {
		t.Errorf("a non-awsvpc service must say so rather than render an empty subnet list\n%s", got)
	}
	if strings.Contains(got, "Public IP") {
		t.Errorf("there is no ENI, so no public-IP flag may be reported\n%s", got)
	}
}

// The default for assignPublicIp depends on how the service was created, so rendering DISABLED for an unanswered field would be a claim about reachability.
func TestServiceOverviewNetworkingEmptyStates(t *testing.T) {
	forceColor(t)
	service, overview, now := serviceFixture()
	service.Network = &aws.ECSAwsVpcConfig{}

	got := utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))

	for _, want := range []string{"Public IP: not reported", "Subnets: none", "Security groups: none"} {
		if !strings.Contains(got, want) {
			t.Errorf("networking section does not contain %q\n%s", want, got)
		}
	}
}

// A ticker re-renders this pane, and rows following the order the API answered in would reshuffle under the cursor between refreshes.
func TestServiceOverviewEventsAreNewestFirst(t *testing.T) {
	forceColor(t)
	service, overview, now := serviceFixture()

	got := utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))

	steady := strings.Index(got, "has reached a steady state.")
	started := strings.Index(got, "has started 1 tasks.")
	if steady == -1 || started == -1 {
		t.Fatalf("both events must render\n%s", got)
	}
	if steady > started {
		t.Errorf("events must render newest first, whatever order DescribeServices answered in\n%s", got)
	}
	// The whole line, not a Contains: the age is what turns an event list into a timeline, and a missing one is invisible to a substring check.
	if line := lineContaining(got, "has reached a steady state."); !strings.HasPrefix(line, "20m ago") {
		t.Errorf("the newest event must open with its age, got %q", line)
	}
}

// ECS opens every message with the service name the pane already carries in its header, which is a fifth of a narrow column spent on a repeat.
func TestServiceOverviewTrimsTheServicePrefixOnlyWhenItMatches(t *testing.T) {
	forceColor(t)
	service, overview, now := serviceFixture()
	when := now.Add(-time.Hour)
	service.Events = []aws.ECSEvent{
		{Message: "(service app-auth) has begun draining connections on 1 tasks.", When: &when},
		{Message: "(service other-service) registered 1 targets.", When: &when},
	}

	got := utils.Decolorise(FormatECSServiceOverview(service, overview, 200, now))

	if strings.Contains(got, "(service app-auth)") {
		t.Errorf("this service's own name must not be repeated on every event line\n%s", got)
	}
	// Trimming is an exact match for THIS service, so anything in another shape is left whole rather than cut on a guess about the format.
	if !strings.Contains(got, "(service other-service) registered 1 targets.") {
		t.Errorf("a message that is not this service's own prefix must be left untouched\n%s", got)
	}
}

func TestServiceOverviewWithoutEvents(t *testing.T) {
	forceColor(t)
	service, overview, now := serviceFixture()
	service.Events = nil

	got := utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))

	if !strings.Contains(got, "no recent events") {
		t.Errorf("an empty event list must say so rather than leave the section blank\n%s", got)
	}
}

// ECS keeps the last hundred events and the pane has room for a handful, so the cap is what stops the newest ones being pushed off by history.
func TestServiceOverviewCapsTheEventsList(t *testing.T) {
	forceColor(t)
	service, overview, now := serviceFixture()
	service.Events = nil
	for i := range 9 {
		when := now.Add(-time.Duration(i) * time.Hour)
		service.Events = append(service.Events, aws.ECSEvent{Message: "event-" + string(rune('a'+i)), When: &when})
	}

	got := utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))

	for _, want := range []string{"event-a", "event-e"} {
		if !strings.Contains(got, want) {
			t.Errorf("the %d newest events must render, %q is missing\n%s", ecsServiceEventsShown, want, got)
		}
	}
	if strings.Contains(got, "event-f") {
		t.Errorf("only the %d newest events may render\n%s", ecsServiceEventsShown, got)
	}
}

// An unpublished series and a genuinely idle service both compute to 0%, and a bar sitting at zero is the more believable of the two lies.
func TestServiceOverviewGaugeReportsNoDataRatherThanZero(t *testing.T) {
	forceColor(t)
	service, overview, now := serviceFixture()
	overview.Metrics = &aws.ECSServiceMetrics{CPUUtilization: aws.MetricPoint{Value: 37.5, At: now, OK: true}}

	got := utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))

	if !strings.Contains(got, "Memory: no data") {
		t.Errorf("an absent series must render no data, never a 0.0%% bar\n%s", got)
	}
	if strings.Contains(got, "0.0%") {
		t.Errorf("no gauge may be drawn from an absent reading\n%s", got)
	}
}

// Each fetch has to cost its own block and not the pane, which is the whole point of the fan-out; and the reason has to stay on screen, because this pane retries on a ticker where a throttle and a denial otherwise look identical.
func TestServiceOverviewSectionsFailIndependently(t *testing.T) {
	forceColor(t)
	service, overview, now := serviceFixture()
	overview.Metrics = nil
	overview.Image = aws.ECSServiceImage{}
	overview.Errs[aws.SectionMetrics] = errors.New("AccessDenied: cloudwatch:GetMetricData")
	overview.Errs[aws.SectionImage] = errors.New("AccessDenied: ecs:DescribeTasks")

	got := utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))

	for _, want := range []string{
		"unavailable: AccessDenied: cloudwatch:GetMetricData",
		"unavailable: AccessDenied: ecs:DescribeTasks",
		// Everything below arrives with DescribeServices, so no fetch failure may take it off the pane.
		"app-auth",
		"Circuit breaker: enabled, rolls back",
		"subnet-0a1b2c3d4e5f60718",
		"has reached a steady state.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview does not contain %q\n%s", want, got)
		}
	}
	// A failed resolution cannot know which of the two labels it would have been, so it must claim neither.
	if strings.Contains(got, "Running image") || strings.Contains(got, "Desired image") {
		t.Errorf("a failed image fetch must not be labelled running or desired\n%s", got)
	}
}

// A hand-built overview with no metrics and a fetch that came back with nothing published are the same thing on screen, and neither is an error to report.
func TestServiceOverviewToleratesAMissingMetricsStruct(t *testing.T) {
	forceColor(t)
	service, overview, now := serviceFixture()
	overview.Metrics = nil

	got := utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))

	if !strings.Contains(got, "CPU:    no data") || !strings.Contains(got, "Memory: no data") {
		t.Errorf("both gauges must read no data\n%s", got)
	}
	if strings.Contains(got, "unavailable") {
		t.Errorf("an absent metrics struct is not a failed fetch and must not be reported as one\n%s", got)
	}
}

// Wrapping is off on an overview, so a line over its budget runs off the pane rather than folding.
func TestServiceOverviewNeverExceedsTheWidth(t *testing.T) {
	forceColor(t)
	service, overview, now := serviceFixture()
	// A long name in the header, which Columns never measures because it spans the full width, and a rollout reason that runs past any column.
	service.Name = "a-very-long-service-name-nobody-should-have-but-someone-does"
	service.Deployments = []aws.ECSDeployment{{
		Status: aws.ECSDeploymentPrimary, RolloutState: aws.ECSRolloutFailed, FailedTasks: 3,
		RolloutStateReason: "ECS deployment circuit breaker: task failed to start and the reason ECS gives runs well past any column this pane can offer it.",
	}}

	for width := 40; width <= 220; width++ {
		for _, line := range strings.Split(FormatECSServiceOverview(service, overview, width, now), "\n") {
			if got := runewidth.StringWidth(utils.Decolorise(line)); got > width {
				t.Fatalf("at width %d a line is %d cells wide: %q", width, got, utils.Decolorise(line))
			}
		}
	}
}

// Mid-rollout a service carries two deployments, and only the PRIMARY one is what it is trying to reach; the other is the revision it is draining away from.
// Listed PRIMARY-second on purpose: a formatter that took whichever deployment came first passes every fixture that happens to be in the right order.
func TestServiceOverviewReadsThePrimaryDeployment(t *testing.T) {
	forceColor(t)
	service, overview, now := serviceFixture()
	settled := now.Add(-3 * 24 * time.Hour)
	service.Deployments = []aws.ECSDeployment{
		{
			Status: "ACTIVE", RolloutState: aws.ECSRolloutCompleted, Created: &settled,
			TaskDefinition: "arn:aws:ecs:eu-west-1:123456789012:task-definition/app-auth:41",
		},
		{
			Status: aws.ECSDeploymentPrimary, RolloutState: "IN_PROGRESS", Created: service.Deployments[0].Created,
			TaskDefinition: "arn:aws:ecs:eu-west-1:123456789012:task-definition/app-auth:42",
		},
	}

	got := utils.Decolorise(FormatECSServiceOverview(service, overview, overviewTestWidth, now))

	for _, want := range []string{"● IN_PROGRESS", "Started:         6h ago", "Task definition: app-auth:42"} {
		if !strings.Contains(got, want) {
			t.Errorf("the deployment section must describe the PRIMARY deployment, %q is missing\n%s", want, got)
		}
	}
	if strings.Contains(got, "● COMPLETED") {
		t.Errorf("the deployment being drained away from must not be the one reported\n%s", got)
	}
}
