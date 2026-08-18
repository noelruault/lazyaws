package ui

import (
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
)

// ecsDrillDown/ecsDrillUp are the pure enter/esc transition core of the ECS panel; wrong transitions show the wrong services or pop esc to the wrong level.
func TestECSDrillDown(t *testing.T) {
	tests := []struct {
		name      string
		start     ecsDrillState
		cluster   string
		service   string
		wantState ecsDrillState
		wantTitle string
	}{
		{
			name:      "clusters to services",
			start:     ecsDrillState{level: ecsLevelClusters},
			cluster:   "prod",
			wantState: ecsDrillState{level: ecsLevelServices, cluster: "prod"},
			wantTitle: "ECS: prod",
		},
		{
			name:      "services to tasks",
			start:     ecsDrillState{level: ecsLevelServices, cluster: "prod"},
			service:   "web",
			wantState: ecsDrillState{level: ecsLevelTasks, cluster: "prod", service: "web"},
			wantTitle: "ECS: prod/web",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, title := ecsDrillDown(tt.start, tt.cluster, tt.service)
			if got != tt.wantState || title != tt.wantTitle {
				t.Errorf("ecsDrillDown(%+v, %q, %q) = (%+v, %q), want (%+v, %q)",
					tt.start, tt.cluster, tt.service, got, title, tt.wantState, tt.wantTitle)
			}
		})
	}
}

func TestECSDrillUp(t *testing.T) {
	tests := []struct {
		name      string
		start     ecsDrillState
		wantState ecsDrillState
		wantTitle string
	}{
		{
			name:      "tasks to services",
			start:     ecsDrillState{level: ecsLevelTasks, cluster: "prod", service: "web"},
			wantState: ecsDrillState{level: ecsLevelServices, cluster: "prod"},
			wantTitle: "ECS: prod",
		},
		{
			name:      "services to clusters",
			start:     ecsDrillState{level: ecsLevelServices, cluster: "prod"},
			wantState: ecsDrillState{},
			wantTitle: "ECS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, title := ecsDrillUp(tt.start)
			if got != tt.wantState || title != tt.wantTitle {
				t.Errorf("ecsDrillUp(%+v) = (%+v, %q), want (%+v, %q)", tt.start, got, title, tt.wantState, tt.wantTitle)
			}
		})
	}
}

// ecsRow's arn/name/status accessors pick the field matching Kind; the wrong branch would show a cluster's ARN as a task's cache key.
func TestECSRowAccessors(t *testing.T) {
	row := &ecsRow{
		Kind:    ecsRowKindService,
		Service: &aws.ECSService{Name: "web", Arn: "arn:svc", Status: "ACTIVE"},
	}

	if got := row.arn(); got != "arn:svc" {
		t.Errorf("arn() = %q, want %q", got, "arn:svc")
	}
	if got := row.name(); got != "web" {
		t.Errorf("name() = %q, want %q", got, "web")
	}
	if got := row.status(); got != "ACTIVE" {
		t.Errorf("status() = %q, want %q", got, "ACTIVE")
	}
}
