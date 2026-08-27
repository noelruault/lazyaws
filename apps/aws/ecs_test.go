package aws

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func statistic(name, value string) ecsTypes.KeyValuePair {
	return ecsTypes.KeyValuePair{Name: &name, Value: &value}
}

// Neither field is returned unless it is asked for, and both come back silently absent rather than as an error, so the request is the only place this can be caught.
func TestClusterDescribeFieldsAsksForStatisticsAndSettings(t *testing.T) {
	asked := map[ecsTypes.ClusterField]bool{}
	for _, f := range clusterDescribeFields() {
		asked[f] = true
	}
	// Pinned to the literals: comparing against the enum the code already uses agrees with whatever it is changed to.
	for _, want := range []ecsTypes.ClusterField{"STATISTICS", "SETTINGS"} {
		if !asked[want] {
			t.Errorf("DescribeClusters does not ask for %s; without it the data reads as zero rather than as missing", want)
		}
	}
}

// The described page is where the Insights setting is read, and the service metrics gate is its only reader, so mapping without recording would lose the extras with nothing failing.
func TestIngestECSClustersMapsAndRecordsTheInsightsSetting(t *testing.T) {
	name, arn, status := "batch-cluster", "arn:aws:ecs:eu-west-1:123:cluster/batch-cluster", "ACTIVE"
	insights := "enabled"
	c := &Client{Region: "eu-west-1", AccountID: "123"}

	got := c.ingestECSClusters([]ecsTypes.Cluster{{
		ClusterName:         &name,
		ClusterArn:          &arn,
		Status:              &status,
		RunningTasksCount:   1,
		ActiveServicesCount: 1,
		Statistics:          []ecsTypes.KeyValuePair{statistic("runningFargateTasksCount", "1")},
		Settings:            []ecsTypes.ClusterSetting{{Name: ecsTypes.ClusterSettingNameContainerInsights, Value: &insights}},
	}})

	if len(got) != 1 {
		t.Fatalf("ingestECSClusters() returned %d clusters, want 1", len(got))
	}
	if got[0].Name != name || got[0].Status != status || got[0].RunningTasksCount != 1 {
		t.Errorf("ingestECSClusters() = %+v, want the described cluster's identity and counts", got[0])
	}
	if got[0].Statistics.RunningFargateTasks != 1 {
		t.Errorf("Statistics = %+v, want the STATISTICS page carried onto the cluster", got[0].Statistics)
	}
	if got[0].ContainerInsights != "enabled" {
		t.Errorf("ContainerInsights = %q, want the SETTINGS entry carried onto the cluster", got[0].ContainerInsights)
	}
	if !ContainerInsightsEnabled(c.clusterInsightsSetting(name)) {
		t.Error("ingesting a cluster must record its Insights setting; nothing else reads it, so the metrics extras would be lost silently")
	}
	if !strings.Contains(got[0].ConsoleURL, name) {
		t.Errorf("ConsoleURL = %q, want the cluster's console link", got[0].ConsoleURL)
	}
}

// A cluster the caller cannot identify the account or region for has no console link, and an empty page is not a cluster.
func TestIngestECSClustersWithoutAccountContext(t *testing.T) {
	name := "prod"
	got := (&Client{}).ingestECSClusters([]ecsTypes.Cluster{{ClusterName: &name}})

	if len(got) != 1 || got[0].ConsoleURL != "" {
		t.Errorf("ingestECSClusters() = %+v, want no console link without a region and account", got)
	}
	if len((&Client{}).ingestECSClusters(nil)) != 0 {
		t.Error("ingestECSClusters(nil) returned clusters")
	}
}

func TestMapClusterStatistics(t *testing.T) {
	// The names and the split are the shape DescribeClusters answers with when asked for STATISTICS.
	got := mapClusterStatistics([]ecsTypes.KeyValuePair{
		statistic("runningFargateTasksCount", "1"),
		statistic("runningEC2TasksCount", "0"),
		statistic("pendingFargateTasksCount", "2"),
		statistic("pendingEC2TasksCount", "3"),
		statistic("activeFargateServiceCount", "1"),
		statistic("activeEC2ServiceCount", "4"),
		statistic("drainingFargateServiceCount", "5"),
		statistic("drainingEC2ServiceCount", "6"),
	})

	want := ECSClusterStatistics{
		RunningEC2Tasks:         0,
		RunningFargateTasks:     1,
		PendingEC2Tasks:         3,
		PendingFargateTasks:     2,
		ActiveEC2Services:       4,
		ActiveFargateServices:   1,
		DrainingEC2Services:     6,
		DrainingFargateServices: 5,
	}
	if got != want {
		t.Errorf("mapClusterStatistics() = %+v, want %+v", got, want)
	}
}

// AWS documents these keys with a leading capital and answers with a leading lowercase; a mapper that matched exactly would read every count as zero against one of the two.
func TestMapClusterStatisticsIgnoresKeyCase(t *testing.T) {
	got := mapClusterStatistics([]ecsTypes.KeyValuePair{statistic("RunningFargateTasksCount", "7")})
	if got.RunningFargateTasks != 7 {
		t.Errorf("RunningFargateTasks = %d, want 7 from a capitalised key", got.RunningFargateTasks)
	}
}

// A cluster described without Include: [STATISTICS] carries no statistics at all, and an unparseable value is not a count.
func TestMapClusterStatisticsToleratesMissingAndUnparseableValues(t *testing.T) {
	if got := mapClusterStatistics(nil); got != (ECSClusterStatistics{}) {
		t.Errorf("mapClusterStatistics(nil) = %+v, want the zero value", got)
	}
	got := mapClusterStatistics([]ecsTypes.KeyValuePair{
		statistic("runningFargateTasksCount", "not-a-number"),
		statistic("pendingFargateTasksCount", "2"),
	})
	if got.RunningFargateTasks != 0 || got.PendingFargateTasks != 2 {
		t.Errorf("mapClusterStatistics() = %+v, want the unparseable key skipped and its neighbour kept", got)
	}
}

func TestContainerInsightsSetting(t *testing.T) {
	value := "enabled"
	other := "off"
	settings := []ecsTypes.ClusterSetting{
		{Name: ecsTypes.ClusterSettingName("somethingElse"), Value: &other},
		{Name: ecsTypes.ClusterSettingNameContainerInsights, Value: &value},
	}
	if got := containerInsightsSetting(settings); got != "enabled" {
		t.Errorf("containerInsightsSetting() = %q, want %q read off the containerInsights entry, not the first one", got, "enabled")
	}
	if got := containerInsightsSetting(nil); got != "" {
		t.Errorf("containerInsightsSetting(nil) = %q, want empty: a cluster described without SETTINGS has an unknown setting, not a disabled one", got)
	}
}

func TestContainerInsightsEnabled(t *testing.T) {
	for setting, want := range map[string]bool{
		"enabled":  true,
		"enhanced": true,
		"ENABLED":  true,
		"disabled": false,
		"":         false,
	} {
		if got := ContainerInsightsEnabled(setting); got != want {
			t.Errorf("ContainerInsightsEnabled(%q) = %v, want %v", setting, got, want)
		}
	}
}

func TestExecECSTask(t *testing.T) {
	c := &Client{Region: "eu-west-1"}
	cmd := c.ExecECSTask("my-cluster", "arn:aws:ecs:eu-west-1:123:task/my-cluster/abc123", "web")

	got := strings.Join(cmd.Args, " ")
	want := "aws ecs execute-command --cluster my-cluster --task arn:aws:ecs:eu-west-1:123:task/my-cluster/abc123 --container web --command /bin/sh --interactive --region eu-west-1"
	if got != want {
		t.Errorf("ExecECSTask() args = %q, want %q", got, want)
	}
}

func TestTaskDefinitionFamily(t *testing.T) {
	tests := []struct {
		arn  string
		want string
	}{
		{arn: "arn:aws:ecs:eu-west-1:123:task-definition/web:7", want: "web"},
		{arn: "arn:aws:ecs:eu-west-1:123:task-definition/web", want: "web"},
		{arn: "web:7", want: "web"},
	}
	for _, tt := range tests {
		if got := TaskDefinitionFamily(tt.arn); got != tt.want {
			t.Errorf("TaskDefinitionFamily(%q) = %q, want %q", tt.arn, got, tt.want)
		}
	}
}

func TestExtractTaskDefRevision(t *testing.T) {
	tests := []struct {
		arn  string
		want int32
	}{
		{arn: "arn:aws:ecs:eu-west-1:123:task-definition/web:7", want: 7},
		{arn: "arn:aws:ecs:eu-west-1:123:task-definition/web", want: 0},
		{arn: "not-an-arn", want: 0},
	}
	for _, tt := range tests {
		if got := extractTaskDefRevision(tt.arn); got != tt.want {
			t.Errorf("extractTaskDefRevision(%q) = %d, want %d", tt.arn, got, tt.want)
		}
	}
}

// Essential is the whole basis for picking the container whose image identifies the task, and only the task definition carries it.
func TestMapECSContainerTakesEssentialFromTheDefinition(t *testing.T) {
	name, image, status := "app-auth", "123456789012.dkr.ecr.eu-west-1.amazonaws.com/app-auth:v1.2.0-develop.0", "RUNNING"
	described := ecsTypes.Container{Name: &name, Image: &image, LastStatus: &status}
	essential := true
	memory := int32(2048)
	definition := ecsTypes.ContainerDefinition{Name: &name, Essential: &essential, Cpu: 1024, Memory: &memory}

	got := mapECSContainer(described, &definition)
	if !got.Essential {
		t.Error("a container its definition marks essential must be mapped essential, or the primary container is picked by position instead")
	}
	if got.ImageURI != image || got.Name != name || got.LastStatus != status {
		t.Errorf("mapECSContainer() = %+v, want the described container's identity, image and status", got)
	}
	if got.CPU != 1.0 || got.MemoryHardMB != 2048 {
		t.Errorf("CPU/memory = %v/%d, want 1 vCPU and 2048 MiB off the definition", got.CPU, got.MemoryHardMB)
	}

	// A task whose definition failed to load still maps, and non-essential is the honest answer rather than a guess.
	withoutDefinition := mapECSContainer(described, nil)
	if withoutDefinition.Essential {
		t.Error("a container with no definition must not be mapped essential")
	}
	if withoutDefinition.ImageURI != image {
		t.Errorf("ImageURI = %q, want the image DescribeTasks carried even with no definition", withoutDefinition.ImageURI)
	}

	notEssential := false
	if mapECSContainer(described, &ecsTypes.ContainerDefinition{Name: &name, Essential: &notEssential}).Essential {
		t.Error("a sidecar its definition marks non-essential must not be mapped essential")
	}
}

// buildECSTask is where a described task meets its definition, and it is the only place that pairs them; a container matched to the wrong definition, or to none, loses the essential flag with nothing failing.
func TestBuildECSTaskPairsContainersWithTheirDefinitions(t *testing.T) {
	app, sidecar := "app-auth", "grafana-alloy-sidecar"
	appImage, sidecarImage := "123456789012.dkr.ecr.eu-west-1.amazonaws.com/app-auth:v1.2.0-develop.0", "grafana/alloy:v1.10.0"
	arn, status := "arn:aws:ecs:eu-west-1:123:task/app-cluster/abc123", "RUNNING"
	tdArn := "arn:aws:ecs:eu-west-1:123:task-definition/app-auth-stage-task:63"
	essential, notEssential := true, false

	// The sidecar is listed first so a mapper keyed on position rather than on name would pick the wrong definition.
	described := ecsTypes.Task{
		TaskArn: &arn, LastStatus: &status, TaskDefinitionArn: &tdArn,
		Containers: []ecsTypes.Container{
			{Name: &sidecar, Image: &sidecarImage},
			{Name: &app, Image: &appImage},
		},
	}
	definition := &ecsTypes.TaskDefinition{ContainerDefinitions: []ecsTypes.ContainerDefinition{
		{Name: &app, Essential: &essential},
		{Name: &sidecar, Essential: &notEssential},
	}}

	task := (&Client{}).buildECSTask(described, "app-cluster", "app-auth", func(string) (*ecsTypes.TaskDefinition, error) {
		return definition, nil
	})

	byName := map[string]ECSContainer{}
	for _, ctn := range task.Containers {
		byName[ctn.Name] = ctn
	}
	if len(byName) != 2 {
		t.Fatalf("built %d containers, want both the app and the sidecar", len(byName))
	}
	if !byName[app].Essential {
		t.Error("the app container's definition marks it essential and the built task lost that")
	}
	if byName[sidecar].Essential {
		t.Error("the sidecar was built essential; its definition says otherwise, and the primary-container pick depends on the difference")
	}

	// Resolving through the real entry point, so the flag reaching the image is what is asserted, not just the field.
	image, ok := runningECSServiceImage([]ECSTask{task})
	if !ok || image.Image != "app-auth:v1.2.0-develop.0" || image.Sidecars != 1 {
		t.Errorf("runningECSServiceImage() = %+v (ok=%v), want the essential container's image with one sidecar", image, ok)
	}
}

// A task whose definition cannot be read must still build with its images; the failure costs the essential flag, not the container.
func TestBuildECSTaskSurvivesAnUnreadableDefinition(t *testing.T) {
	app := "app-auth"
	appImage := "123456789012.dkr.ecr.eu-west-1.amazonaws.com/app-auth:v1.2.0-develop.0"
	arn, status := "arn:aws:ecs:eu-west-1:123:task/app-cluster/abc123", "RUNNING"

	task := (&Client{}).buildECSTask(
		ecsTypes.Task{TaskArn: &arn, LastStatus: &status, Containers: []ecsTypes.Container{{Name: &app, Image: &appImage}}},
		"app-cluster", "app-auth",
		func(string) (*ecsTypes.TaskDefinition, error) { return nil, errors.New("access denied") },
	)

	if len(task.Containers) != 1 || task.Containers[0].ImageURI != appImage {
		t.Fatalf("built %+v, want the container and its image despite the definition failing", task.Containers)
	}
	if task.Containers[0].Essential {
		t.Error("a container built without its definition must not claim to be essential")
	}
	if image, ok := runningECSServiceImage([]ECSTask{task}); !ok || image.Image != "app-auth:v1.2.0-develop.0" {
		t.Errorf("runningECSServiceImage() = %+v (ok=%v), want the first container as the fallback primary", image, ok)
	}
}

// The PRIMARY deployment's revision is what the desired-image fallback reads, and nothing else carries it.
func TestMapECSDeploymentCarriesItsTaskDefinition(t *testing.T) {
	status, taskDef := "PRIMARY", "arn:aws:ecs:eu-west-1:123:task-definition/web:8"
	created := time.Date(2026, 8, 27, 17, 43, 0, 0, time.UTC)

	got := mapECSDeployment(ecsTypes.Deployment{
		Status: &status, TaskDefinition: &taskDef, DesiredCount: 2, RunningCount: 1, PendingCount: 1, CreatedAt: &created,
	})

	if got.TaskDefinition != taskDef {
		t.Errorf("TaskDefinition = %q, want %q; without it the desired-image fallback silently reads the service's own revision", got.TaskDefinition, taskDef)
	}
	if got.Status != "PRIMARY" || got.Desired != 2 || got.Running != 1 || got.Pending != 1 {
		t.Errorf("mapECSDeployment() = %+v, want the described counts and status", got)
	}
}

func TestMapTaskDefinitionContainerCarriesEssentialAndImage(t *testing.T) {
	name, image := "app-auth", "123456789012.dkr.ecr.eu-west-1.amazonaws.com/app-auth:v1.2.0-develop.0"
	essential := true
	envName, envValue := "STAGE", "staging"

	got := mapTaskDefinitionContainer(ecsTypes.ContainerDefinition{
		Name:        &name,
		Image:       &image,
		Essential:   &essential,
		Environment: []ecsTypes.KeyValuePair{{Name: &envName, Value: &envValue}},
	})

	if !got.Essential {
		t.Error("an essential container definition must map essential, or the desired image is picked by position instead")
	}
	if got.Image != image || got.Name != name || got.Environment["STAGE"] != "staging" {
		t.Errorf("mapTaskDefinitionContainer() = %+v, want the definition's name, image and environment", got)
	}

	notEssential := false
	if mapTaskDefinitionContainer(ecsTypes.ContainerDefinition{Name: &name, Essential: &notEssential}).Essential {
		t.Error("a non-essential container definition must not map essential")
	}
}

func TestShortImageRef(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  string
	}{
		{"ecr host is dropped", "123456789012.dkr.ecr.eu-west-1.amazonaws.com/app-auth:v1.2.0-develop.0", "app-auth:v1.2.0-develop.0"},
		{"docker hub namespace is not a host", "grafana/alloy:v1.10.0", "grafana/alloy:v1.10.0"},
		{"bare image", "nginx:latest", "nginx:latest"},
		{"host with a port", "localhost:5000/app:1", "app:1"},
		{"plain localhost is a host", "localhost/app:1", "app:1"},
		{"nested repository keeps its path", "myregistry.io/team/app:1", "team/app:1"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShortImageRef(tt.image); got != tt.want {
				t.Errorf("ShortImageRef(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}

func container(name, image string, essential bool) ECSContainer {
	return ECSContainer{Name: name, ImageURI: image, Essential: essential}
}

func TestPrimaryECSContainerPrefersTheEssentialOne(t *testing.T) {
	// The sidecar is listed first on purpose: position must not be what decides.
	containers := []ECSContainer{
		container("grafana-alloy-sidecar", "grafana/alloy:v1.10.0", false),
		container("app-auth", "123456789012.dkr.ecr.eu-west-1.amazonaws.com/app-auth:v1.2.0-develop.0", true),
	}

	got, ok := primaryECSContainer(containers)
	if !ok || got.Name != "app-auth" {
		t.Errorf("primaryECSContainer() = %q (ok=%v), want the essential container regardless of its position", got.Name, ok)
	}
}

// A task whose definition could not be read leaves every container non-essential; the first one is still a better answer than none.
func TestPrimaryECSContainerFallsBackToTheFirst(t *testing.T) {
	got, ok := primaryECSContainer([]ECSContainer{container("web", "web:1", false), container("side", "side:1", false)})
	if !ok || got.Name != "web" {
		t.Errorf("primaryECSContainer() = %q (ok=%v), want the first container when none is marked essential", got.Name, ok)
	}
	if _, ok := primaryECSContainer(nil); ok {
		t.Error("primaryECSContainer(nil) reported a container")
	}
}

func taskTime(min int) *time.Time {
	t := time.Date(2026, 8, 27, 17, min, 0, 0, time.UTC)
	return &t
}

func TestRunningECSServiceImageSummarizesSidecars(t *testing.T) {
	tasks := []ECSTask{{
		Status: "RUNNING",
		Containers: []ECSContainer{
			container("app-auth", "123456789012.dkr.ecr.eu-west-1.amazonaws.com/app-auth:v1.2.0-develop.0", true),
			container("grafana-alloy-sidecar", "grafana/alloy:v1.10.0", false),
		},
		StartedAt: taskTime(10),
	}}

	got, ok := runningECSServiceImage(tasks)
	if !ok {
		t.Fatal("runningECSServiceImage() found no image on a running task")
	}
	want := ECSServiceImage{Image: "app-auth:v1.2.0-develop.0", Sidecars: 1}
	if got != want {
		t.Errorf("runningECSServiceImage() = %+v, want %+v", got, want)
	}
}

// Two tasks differing only in their sidecar must still report the same primary image, and a single-container task must report no sidecars.
func TestRunningECSServiceImageIgnoresSidecarDifferences(t *testing.T) {
	primary := container("app-auth", "123456789012.dkr.ecr.eu-west-1.amazonaws.com/app-auth:v1.2.0-develop.0", true)

	withSidecar, _ := runningECSServiceImage([]ECSTask{{
		Status:     "RUNNING",
		Containers: []ECSContainer{primary, container("grafana-alloy-sidecar", "grafana/alloy:v1.10.0", false)},
		StartedAt:  taskTime(10),
	}})
	alone, _ := runningECSServiceImage([]ECSTask{{
		Status:     "RUNNING",
		Containers: []ECSContainer{primary},
		StartedAt:  taskTime(10),
	}})

	if withSidecar.Image != alone.Image {
		t.Errorf("primary image = %q with a sidecar and %q without; the sidecar must not change what the service is running", withSidecar.Image, alone.Image)
	}
	if withSidecar.Sidecars != 1 || alone.Sidecars != 0 {
		t.Errorf("sidecar counts = %d and %d, want 1 and 0", withSidecar.Sidecars, alone.Sidecars)
	}
}

// Mid-rollout a service runs two revisions at once; the newest task is the one rolling out, and the answer must not depend on the order DescribeTasks replied in.
func TestNewestECSTaskSkipsStoppedTasksAndIgnoresResponseOrder(t *testing.T) {
	old := ECSTask{ID: "old", Status: "RUNNING", StartedAt: taskTime(10)}
	fresh := ECSTask{ID: "fresh", Status: "RUNNING", StartedAt: taskTime(40)}
	stoppedAndNewer := ECSTask{ID: "stopped", Status: "STOPPED", StartedAt: taskTime(50)}

	for _, order := range [][]ECSTask{
		{old, fresh, stoppedAndNewer},
		{stoppedAndNewer, fresh, old},
		{fresh, stoppedAndNewer, old},
	} {
		got, ok := newestECSTask(order)
		if !ok || got.ID != "fresh" {
			t.Errorf("newestECSTask() = %q (ok=%v), want the newest RUNNING task", got.ID, ok)
		}
	}

	if _, ok := newestECSTask([]ECSTask{stoppedAndNewer}); ok {
		t.Error("newestECSTask() picked a STOPPED task; a stopped task is not what the service is running")
	}
	if _, ok := newestECSTask(nil); ok {
		t.Error("newestECSTask(nil) reported a task")
	}
}

// A task still provisioning has a creation time and no start time, and it must not sort as the oldest thing in the list.
func TestNewestECSTaskFallsBackToCreationTime(t *testing.T) {
	started := ECSTask{ID: "started", Status: "RUNNING", StartedAt: taskTime(10)}
	created := ECSTask{ID: "created", Status: "RUNNING", CreatedAt: taskTime(40)}

	if got, _ := newestECSTask([]ECSTask{started, created}); got.ID != "created" {
		t.Errorf("newestECSTask() = %q, want the task created later even though it has not recorded a start", got.ID)
	}
}

// A service with nothing running still has an intended image, and it must be labelled as intended rather than reported as live.
func TestDesiredECSServiceImageIsMarkedDesired(t *testing.T) {
	detail := &ECSTaskDefinitionDetail{Containers: []ECSTaskDefinitionContainer{
		{Name: "grafana-alloy-sidecar", Image: "grafana/alloy:v1.10.0"},
		{Name: "app-auth", Image: "123456789012.dkr.ecr.eu-west-1.amazonaws.com/app-auth:v1.2.0-develop.0", Essential: true},
	}}

	got, ok := desiredECSServiceImage(detail)
	if !ok {
		t.Fatal("desiredECSServiceImage() found no image in a task definition with containers")
	}
	want := ECSServiceImage{Image: "app-auth:v1.2.0-develop.0", Sidecars: 1, Desired: true}
	if got != want {
		t.Errorf("desiredECSServiceImage() = %+v, want %+v", got, want)
	}

	if _, ok := desiredECSServiceImage(&ECSTaskDefinitionDetail{}); ok {
		t.Error("desiredECSServiceImage() reported an image for a task definition with no containers")
	}
	if _, ok := desiredECSServiceImage(nil); ok {
		t.Error("desiredECSServiceImage(nil) reported an image")
	}
}

// Zero running tasks is what sends the resolver to the task definition, so the empty task list must not resolve to a running image.
func TestRunningECSServiceImageFindsNothingWithoutARunningTask(t *testing.T) {
	for _, tasks := range [][]ECSTask{
		nil,
		{{Status: "STOPPED", Containers: []ECSContainer{container("web", "web:1", true)}}},
		{{Status: "RUNNING"}},
	} {
		if got, ok := runningECSServiceImage(tasks); ok {
			t.Errorf("runningECSServiceImage(%+v) = %+v, want no image so the desired-image fallback runs", tasks, got)
		}
	}
}

// During a rollout the service's own taskDefinition field has already moved on; the PRIMARY deployment is what the service is trying to reach.
func TestServiceTaskDefinitionPrefersThePrimaryDeployment(t *testing.T) {
	s := &ECSService{
		TaskDefinition: "arn:aws:ecs:eu-west-1:123:task-definition/web:7",
		Deployments: []ECSDeployment{
			{Status: "ACTIVE", TaskDefinition: "arn:aws:ecs:eu-west-1:123:task-definition/web:6"},
			{Status: "PRIMARY", TaskDefinition: "arn:aws:ecs:eu-west-1:123:task-definition/web:8"},
		},
	}
	if got := serviceTaskDefinition(s); got != "arn:aws:ecs:eu-west-1:123:task-definition/web:8" {
		t.Errorf("serviceTaskDefinition() = %q, want the PRIMARY deployment's revision", got)
	}

	noDeployments := &ECSService{TaskDefinition: "arn:aws:ecs:eu-west-1:123:task-definition/web:7"}
	if got := serviceTaskDefinition(noDeployments); got != noDeployments.TaskDefinition {
		t.Errorf("serviceTaskDefinition() = %q, want the service's own revision when no deployment names one", got)
	}
}

// A revisionless reference resolves to whatever is latest at call time, so caching one would keep serving a revision that has since been superseded.
func TestTaskDefCacheOnlyHoldsPinnedRevisions(t *testing.T) {
	pinned := "arn:aws:ecs:eu-west-1:123:task-definition/web:7"
	floating := "arn:aws:ecs:eu-west-1:123:task-definition/web"

	c := &Client{}
	detail := &ECSTaskDefinitionDetail{Family: "web", Revision: 7}
	c.cacheTaskDef(pinned, detail)
	c.cacheTaskDef(floating, detail)

	if got, ok := c.cachedTaskDef(pinned); !ok || got != detail {
		t.Errorf("cachedTaskDef(%q) = %v (ok=%v), want the cached revision", pinned, got, ok)
	}
	if _, ok := c.cachedTaskDef(floating); ok {
		t.Errorf("cachedTaskDef(%q) returned a hit; an unpinned reference means latest and must be re-read", floating)
	}
	if !taskDefIsImmutable(pinned) || taskDefIsImmutable(floating) || taskDefIsImmutable("web") {
		t.Error("taskDefIsImmutable must hold only for a reference that pins a revision")
	}
}

func TestChunkStrings(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		size  int
		want  [][]string
	}{
		{name: "empty", items: nil, size: 100, want: nil},
		{name: "non-positive size", items: []string{"a"}, size: 0, want: nil},
		{name: "under one chunk", items: []string{"a", "b"}, size: 3, want: [][]string{{"a", "b"}}},
		{name: "exact multiple", items: []string{"a", "b", "c", "d"}, size: 2, want: [][]string{{"a", "b"}, {"c", "d"}}},
		{name: "remainder", items: []string{"a", "b", "c"}, size: 2, want: [][]string{{"a", "b"}, {"c"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunkStrings(tt.items, tt.size)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("chunkStrings(%v, %d) = %v, want %v", tt.items, tt.size, got, tt.want)
			}
		})
	}
}
