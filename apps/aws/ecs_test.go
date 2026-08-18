package aws

import (
	"reflect"
	"strings"
	"testing"
)

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
