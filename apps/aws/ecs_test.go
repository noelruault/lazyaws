package aws

import (
	"reflect"
	"strings"
	"testing"

	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func statistic(name, value string) ecsTypes.KeyValuePair {
	return ecsTypes.KeyValuePair{Name: &name, Value: &value}
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
