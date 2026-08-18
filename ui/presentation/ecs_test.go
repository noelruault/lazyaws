package presentation

import (
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

func decoloriseAll(cells []string) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = utils.Decolorise(c)
	}
	return out
}

func TestGetECSClusterDisplayStrings(t *testing.T) {
	c := &aws.ECSCluster{
		Name: "prod", Status: "ACTIVE",
		RunningTasksCount: 3, PendingTasksCount: 1, ActiveServicesCount: 2,
	}
	got := decoloriseAll(GetECSClusterDisplayStrings(c))
	want := []string{"▶", "prod", "2 services", "3 running / 1 pending"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cell %d = %q, want %q (row %v)", i, got[i], want[i], got)
		}
	}
}

func TestGetECSServiceDisplayStrings(t *testing.T) {
	s := &aws.ECSService{Name: "web", Status: "ACTIVE", LaunchType: "FARGATE", DesiredCount: 2, RunningCount: 2}
	got := decoloriseAll(GetECSServiceDisplayStrings(s))
	want := []string{"▶", "web", "FARGATE", "2/2"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cell %d = %q, want %q (row %v)", i, got[i], want[i], got)
		}
	}
}

func TestGetECSTaskDisplayStrings(t *testing.T) {
	tsk := &aws.ECSTask{ID: "abc123", Status: "RUNNING", LaunchType: "FARGATE"}
	got := decoloriseAll(GetECSTaskDisplayStrings(tsk))
	want := []string{"▶", "abc123", "FARGATE"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cell %d = %q, want %q (row %v)", i, got[i], want[i], got)
		}
	}
}
