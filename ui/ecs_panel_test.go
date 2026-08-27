package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/jesseduffield/gocui"
	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/apps/aws"
)

func TestFormatECSClusterConfig(t *testing.T) {
	c := &aws.ECSCluster{Name: "prod", Status: "ACTIVE", RunningTasksCount: 2, ConsoleURL: "https://example/console"}
	data := &aws.ECSClusterData{
		Services: []aws.ECSService{
			{Name: "web", Status: "ACTIVE", DesiredCount: 2, RunningCount: 2},
		},
	}

	out := formatECSClusterConfig(c, data, nil)

	for _, want := range []string{"prod", "https://example/console", "web"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatECSClusterConfig() missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatECSClusterConfigNoServices(t *testing.T) {
	c := &aws.ECSCluster{Name: "empty"}
	data := &aws.ECSClusterData{}

	out := formatECSClusterConfig(c, data, nil)

	if !strings.Contains(out, "none") {
		t.Errorf("formatECSClusterConfig() with no services should mention \"none\", got:\n%s", out)
	}
}

func TestFormatECSClusterConfigWithInsights(t *testing.T) {
	c := &aws.ECSCluster{Name: "prod"}
	data := &aws.ECSClusterData{}
	insights := &aws.ECSContainerInsights{CPUPercent: 42.5, MemPercent: 10}

	out := formatECSClusterConfig(c, data, insights)

	if !strings.Contains(out, "42.5%") {
		t.Errorf("formatECSClusterConfig() missing CPU percent in:\n%s", out)
	}
	if !strings.Contains(out, "10.0%") {
		t.Errorf("formatECSClusterConfig() missing memory percent in:\n%s", out)
	}
}

func TestFormatECSClusterConfigNilInsights(t *testing.T) {
	c := &aws.ECSCluster{Name: "prod"}
	data := &aws.ECSClusterData{}

	out := formatECSClusterConfig(c, data, nil)

	if !strings.Contains(out, "n/a") {
		t.Errorf("formatECSClusterConfig() with nil insights should show \"n/a\", got:\n%s", out)
	}
}

func TestFormatECSContainerInstances(t *testing.T) {
	instances := []aws.ECSContainerInstance{
		{Ec2InstanceID: "i-abc123", Status: "ACTIVE", AgentConnected: true, AgentVersion: "1.80.0", RunningTasksCount: 3, PendingTasksCount: 1},
		{Ec2InstanceID: "i-def456", Status: "DRAINING", AgentConnected: false, AgentVersion: "1.79.0"},
	}

	out := formatECSContainerInstances(instances)

	for _, want := range []string{"i-abc123", "ACTIVE", "connected", "i-def456", "DRAINING", "disconnected"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatECSContainerInstances() missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatECSContainerInstancesEmpty(t *testing.T) {
	out := formatECSContainerInstances(nil)

	if !strings.Contains(out, "Fargate-only") {
		t.Errorf("formatECSContainerInstances(nil) should mention Fargate-only cluster, got:\n%s", out)
	}
}

func TestFormatECSServiceConfigWithTargetHealth(t *testing.T) {
	s := &aws.ECSService{
		Name: "web",
		LoadBalancers: []aws.ECSLoadBalancer{
			{Type: "Load Balancer", Name: "web-alb", TargetGroup: "web-tg", TargetGroupArn: "arn:aws:elasticloadbalancing:tg/web-tg"},
		},
	}
	health := map[string][]aws.ECSTargetHealth{
		"arn:aws:elasticloadbalancing:tg/web-tg": {
			{TargetID: "10.0.0.1", Port: 80, State: "unhealthy", Reason: "Target.Timeout"},
		},
	}

	out := formatECSServiceConfig(s, nil, aws.ECSServiceImage{}, health)

	for _, want := range []string{"web-tg", "10.0.0.1", "Target.Timeout"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatECSServiceConfig() missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatECSServiceConfigNoLoadBalancers(t *testing.T) {
	s := &aws.ECSService{Name: "web"}

	out := formatECSServiceConfig(s, nil, aws.ECSServiceImage{}, nil)

	if !strings.Contains(out, "none") {
		t.Errorf("formatECSServiceConfig() with no load balancers should mention \"none\", got:\n%s", out)
	}
}

// An idle service reads 0.0%, and a service CloudWatch never answered for reads "no data"; the old Insights percentages could not tell those apart because the reservation they divided by was absent exactly when the data was.
func TestFormatECSServiceConfigSeparatesAnIdleServiceFromAnUnmeasuredOne(t *testing.T) {
	s := &aws.ECSService{Name: "web"}
	at := time.Date(2026, 8, 27, 17, 43, 0, 0, time.UTC)

	out := formatECSServiceConfig(s, &aws.ECSServiceMetrics{
		CPUUtilization: aws.MetricPoint{Value: 0, At: at, OK: true},
	}, aws.ECSServiceImage{}, nil)

	if !strings.Contains(out, "0.0% (1-min avg @ 17:43Z)") {
		t.Errorf("formatECSServiceConfig() should stamp a measured zero with its publish time, got:\n%s", out)
	}
	if !strings.Contains(out, "no data") {
		t.Errorf("formatECSServiceConfig() should render the unanswered memory metric as \"no data\", got:\n%s", out)
	}
}

// The reservations only exist where Container Insights is on; a service without them must not grow empty rows for them.
func TestFormatECSServiceConfigAddsInsightsRowsOnlyWhenPresent(t *testing.T) {
	s := &aws.ECSService{Name: "web"}
	at := time.Date(2026, 8, 27, 17, 43, 0, 0, time.UTC)

	without := formatECSServiceConfig(s, &aws.ECSServiceMetrics{CPUUtilization: aws.MetricPoint{Value: 1.12, At: at, OK: true}}, aws.ECSServiceImage{}, nil)
	for _, absent := range []string{"CPU reserved", "Mem reserved", "vCPU", "MiB"} {
		if strings.Contains(without, absent) {
			t.Errorf("formatECSServiceConfig() shows %q with no Insights data, got:\n%s", absent, without)
		}
	}

	with := formatECSServiceConfig(s, &aws.ECSServiceMetrics{
		CPUUtilization:   aws.MetricPoint{Value: 1.12, At: at, OK: true},
		InsightsCPUUsed:  aws.MetricPoint{Value: 11.5, At: at, OK: true},
		InsightsCPUTotal: aws.MetricPoint{Value: 1024, At: at, OK: true},
		InsightsMemUsed:  aws.MetricPoint{Value: 285, At: at, OK: true},
		InsightsMemTotal: aws.MetricPoint{Value: 2048, At: at, OK: true},
	}, aws.ECSServiceImage{}, nil)
	for _, want := range []string{"1024 (1.00 vCPU)", "12 (0.01 vCPU)", "2048 MiB", "285 MiB"} {
		if !strings.Contains(with, want) {
			t.Errorf("formatECSServiceConfig() missing %q with Insights data, got:\n%s", want, with)
		}
	}
}

// The pane must never let an intended image read as a live one, and the label is the only thing that says which it is.
func TestFormatECSServiceConfigLabelsTheImageRunningOrDesired(t *testing.T) {
	s := &aws.ECSService{Name: "web"}

	running := formatECSServiceConfig(s, nil, aws.ECSServiceImage{Image: "app-auth:v1.2.0-develop.0", Sidecars: 1}, nil)
	if !strings.Contains(running, "Running image") || !strings.Contains(running, "app-auth:v1.2.0-develop.0 (+1 sidecar)") {
		t.Errorf("formatECSServiceConfig() should label a live image as running and summarize its sidecar, got:\n%s", running)
	}
	if strings.Contains(running, "Desired image") {
		t.Errorf("formatECSServiceConfig() labelled a running image as desired, got:\n%s", running)
	}

	desired := formatECSServiceConfig(s, nil, aws.ECSServiceImage{Image: "app-auth:v1.2.0-develop.0", Desired: true}, nil)
	if !strings.Contains(desired, "Desired image") {
		t.Errorf("formatECSServiceConfig() should label a task-definition image as desired, got:\n%s", desired)
	}
	if strings.Contains(desired, "Running image") {
		t.Errorf("formatECSServiceConfig() labelled a desired image as running; a service with nothing up is not serving it, got:\n%s", desired)
	}
}

func TestFormatECSServiceConfigWithoutMetrics(t *testing.T) {
	out := formatECSServiceConfig(&aws.ECSService{Name: "web"}, nil, aws.ECSServiceImage{}, nil)

	if !strings.Contains(out, "n/a") {
		t.Errorf("formatECSServiceConfig() with a failed metrics fetch should show \"n/a\", got:\n%s", out)
	}
}

func TestFormatECSServiceScalingNone(t *testing.T) {
	out := formatECSServiceScaling(nil)

	if !strings.Contains(out, "no Application Auto Scaling") {
		t.Errorf("formatECSServiceScaling(nil) should say no auto scaling, got:\n%s", out)
	}
}

func TestFormatECSServiceScalingNoPolicies(t *testing.T) {
	out := formatECSServiceScaling(&aws.ECSServiceAutoScaling{MinCapacity: 2, MaxCapacity: 10})

	for _, want := range []string{"2", "10", "none"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatECSServiceScaling() missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatECSServiceScalingWithPolicies(t *testing.T) {
	scaling := &aws.ECSServiceAutoScaling{
		MinCapacity: 1,
		MaxCapacity: 5,
		Policies: []aws.ECSScalingPolicy{
			{Name: "cpu-target", Type: "TargetTrackingScaling", TargetMetric: "ECSServiceAverageCPUUtilization", TargetValue: 60, ScaleInCooldownSecs: 60, ScaleOutCooldownSecs: 30},
			{Name: "step-out", Type: "StepScaling", StepAdjustments: 2, ScaleOutCooldownSecs: 120},
		},
	}

	out := formatECSServiceScaling(scaling)

	for _, want := range []string{"cpu-target", "60.0", "ECSServiceAverageCPUUtilization", "step-out", "2 step adjustment"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatECSServiceScaling() missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatECSServiceDeploymentsCircuitBreaker(t *testing.T) {
	s := &aws.ECSService{Name: "web", CircuitBreakerEnabled: true, CircuitBreakerRollback: true}

	out := formatECSServiceDeployments(s, nil)

	if !strings.Contains(out, "Circuit breaker: enabled (rollback enabled)") {
		t.Errorf("formatECSServiceDeployments() missing enabled circuit breaker line, got:\n%s", out)
	}
}

func TestFormatECSServiceDeploymentsCircuitBreakerDisabled(t *testing.T) {
	s := &aws.ECSService{Name: "web"}

	out := formatECSServiceDeployments(s, nil)

	if !strings.Contains(out, "Circuit breaker: disabled") {
		t.Errorf("formatECSServiceDeployments() missing disabled circuit breaker line, got:\n%s", out)
	}
}

func TestFormatECSServiceDeploymentsCodeDeploy(t *testing.T) {
	s := &aws.ECSService{Name: "web", DeploymentController: ecsCodeDeployControllerType}
	created := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	cd := &aws.ECSCodeDeployStatus{
		ApplicationName:      "AppECS-prod-web",
		DeploymentGroupName:  "DgpECS-prod-web",
		LastSuccessfulStatus: "Succeeded",
		LastSuccessfulAt:     &created,
	}

	out := formatECSServiceDeployments(s, cd)

	for _, want := range []string{"AppECS-prod-web", "DgpECS-prod-web", "Succeeded", "2026-07-01"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatECSServiceDeployments() missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatECSServiceDeploymentsCodeDeployNotFound(t *testing.T) {
	s := &aws.ECSService{Name: "web", DeploymentController: ecsCodeDeployControllerType}

	out := formatECSServiceDeployments(s, nil)

	if !strings.Contains(out, "no matching deployment group found") {
		t.Errorf("formatECSServiceDeployments() should report missing CodeDeploy group, got:\n%s", out)
	}
}

func TestFormatECSTaskConfig(t *testing.T) {
	task := &aws.ECSTask{
		ID: "abc123", Status: "RUNNING", ConsoleURL: "https://example/task",
		Config: aws.ECSTaskConfig{LaunchType: "FARGATE", TaskDefinition: "web:3"},
		Containers: []aws.ECSContainer{
			{Name: "web", LastStatus: "RUNNING", ImageURI: "nginx:latest",
				Ports: []aws.ECSPortMapping{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}}},
		},
		Attachments: []aws.ECSAttachment{{Type: "ElasticNetworkInterface", Details: map[string]string{"subnetId": "subnet-1"}}},
	}

	out := formatECSTaskConfig(task)

	for _, want := range []string{"abc123", "FARGATE", "web:3", "nginx:latest", "host:8080", "container:80/tcp", "ElasticNetworkInterface"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatECSTaskConfig() missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatECSLogStreams(t *testing.T) {
	base := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	streams := []aws.ECSLogStream{
		{Container: "web", Events: []aws.ECSLogEvent{
			{Timestamp: base.Add(2 * time.Second), Message: "second"},
			{Timestamp: base, Message: "first"},
		}},
	}

	out := formatECSLogStreams(streams)

	firstIdx := strings.Index(out, "first")
	secondIdx := strings.Index(out, "second")
	if firstIdx == -1 || secondIdx == -1 || firstIdx > secondIdx {
		t.Errorf("formatECSLogStreams() did not sort events chronologically:\n%s", out)
	}
	if !strings.Contains(out, "web") {
		t.Errorf("formatECSLogStreams() missing container name in:\n%s", out)
	}
}

func TestFormatECSLogStreamsNone(t *testing.T) {
	if got := formatECSLogStreams(nil); !strings.Contains(got, "no logs configured") {
		t.Errorf("formatECSLogStreams(nil) = %q, want a \"no logs configured\" message", got)
	}
}

func TestDiffTaskDefinitions(t *testing.T) {
	older := &aws.ECSTaskDefinitionDetail{
		CPU: "256", Memory: "512",
		Containers: []aws.ECSTaskDefinitionContainer{
			{Name: "web", Image: "app:1.0", CPU: 128, Memory: 256, Environment: map[string]string{"LOG_LEVEL": "info"}},
			{Name: "sidecar", Image: "proxy:1.0"},
		},
	}
	newer := &aws.ECSTaskDefinitionDetail{
		CPU: "512", Memory: "512",
		Containers: []aws.ECSTaskDefinitionContainer{
			{Name: "web", Image: "app:2.0", CPU: 128, Memory: 512, Environment: map[string]string{"LOG_LEVEL": "debug", "NEW_VAR": "x"}},
			{Name: "collector", Image: "otel:1.0"},
		},
	}

	lines := diffTaskDefinitions(older, newer)
	out := strings.Join(lines, "\n")

	for _, want := range []string{
		"task cpu: 256 -> 512",
		"container web image: app:1.0 -> app:2.0",
		"container web memory: 256 -> 512",
		"container web env LOG_LEVEL: info -> debug",
		"container web env NEW_VAR: added (x)",
		"container sidecar: removed (was proxy:1.0)",
		"container collector: added (otel:1.0)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("diffTaskDefinitions() missing %q in:\n%s", want, out)
		}
	}
}

func TestDiffTaskDefinitionsNoChanges(t *testing.T) {
	rev := &aws.ECSTaskDefinitionDetail{CPU: "256", Memory: "512", Containers: []aws.ECSTaskDefinitionContainer{
		{Name: "web", Image: "app:1.0"},
	}}
	if lines := diffTaskDefinitions(rev, rev); len(lines) != 0 {
		t.Errorf("diffTaskDefinitions(x, x) = %v, want no differences", lines)
	}
}

func TestFormatECSTaskDefDiff(t *testing.T) {
	revisions := []aws.ECSTaskDefinitionRevision{
		{Arn: "arn:aws:ecs:x:y:task-definition/web:3", Revision: 3},
		{Arn: "arn:aws:ecs:x:y:task-definition/web:2", Revision: 2},
	}
	current := &aws.ECSTaskDefinitionDetail{CPU: "256", Containers: []aws.ECSTaskDefinitionContainer{{Name: "web", Image: "app:2.0"}}}
	previous := &aws.ECSTaskDefinitionDetail{CPU: "128", Containers: []aws.ECSTaskDefinitionContainer{{Name: "web", Image: "app:1.0"}}}

	out := formatECSTaskDefDiff("web", revisions, revisions[0].Arn, current, previous)

	for _, want := range []string{"Family: web", "rev 3", "rev 2", "task cpu: 128 -> 256", "app:1.0 -> app:2.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatECSTaskDefDiff() missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatECSTaskDefDiffNoPrevious(t *testing.T) {
	revisions := []aws.ECSTaskDefinitionRevision{{Arn: "arn:x:1", Revision: 1}}
	current := &aws.ECSTaskDefinitionDetail{}

	out := formatECSTaskDefDiff("web", revisions, revisions[0].Arn, current, nil)

	if !strings.Contains(out, "no previous revision") {
		t.Errorf("formatECSTaskDefDiff() = %q, want a \"no previous revision\" message", out)
	}
}

// resizeView gives a view a real width, which the headless harness otherwise leaves at the 10-cell placeholder createAllViews sets, too narrow for any row assertion to mean anything.
func resizeView(t *testing.T, g *gocui.Gui, name string, width, height int) {
	t.Helper()

	set := func(x1, y1 int) error {
		_, err := g.SetView(name, 0, 0, x1, y1, 0)
		if err != nil && err.Error() != gocui.ErrUnknownView.Error() {
			return err
		}
		return nil
	}
	run(t, g, func() error { return set(width+1, height+1) })
	t.Cleanup(func() { run(t, g, func() error { return set(10, 10) }) })
}

// The whole point of laying the side panels out with RenderTableFit is that a long name cannot push the columns that identify a row off the right-hand edge.
func TestECSPanelRendersClusterRowsInsideTheViewWidth(t *testing.T) {
	gui, g := newHeadlessGui(t)
	resizeView(t, g, "ecs", 60, 20)

	run(t, g, func() error {
		gui.Panels.ECS.SetItems([]*ecsRow{
			{Kind: ecsRowKindCluster, Cluster: &aws.ECSCluster{Name: "prod", Status: "ACTIVE", RunningTasksCount: 3, ActiveServicesCount: 2}},
			{Kind: ecsRowKindCluster, Cluster: &aws.ECSCluster{
				Name: strings.Repeat("very-long-cluster-name-", 5), Status: "ACTIVE", PendingTasksCount: 1, ActiveServicesCount: 9,
			}},
		})
		return gui.Panels.ECS.RerenderList()
	})

	width := ask(g, func() int { return gui.Views.ECS.InnerWidth() })
	buffer := ask(g, func() string { return gui.Views.ECS.Buffer() })

	for _, line := range strings.Split(strings.TrimRight(buffer, "\n"), "\n") {
		if got := runewidth.StringWidth(line); got > width {
			t.Errorf("line %q is %d cells wide, want at most %d", line, got, width)
		}
	}
	// The badge is the rightmost column, so it is the first thing an overrunning name would have cost.
	for _, want := range []string{"prod", "● healthy", "● deploying", "3 running / 0 pending"} {
		if !strings.Contains(buffer, want) {
			t.Errorf("ECS view = %q, want it to still show %q", buffer, want)
		}
	}
	if !strings.Contains(buffer, "…") {
		t.Errorf("ECS view = %q, want the over-long name cut with an ellipsis", buffer)
	}
}
