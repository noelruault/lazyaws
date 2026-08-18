package ui

import (
	"strings"
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/resources"
)

func ecsAt(t *testing.T, gui *Gui, drill ecsDrillState, row *ecsRow) []resources.Action {
	t.Helper()

	return ask(gui.g, func() []resources.Action {
		gui.ecsDrill = drill
		gui.Panels.ECS.SetItems([]*ecsRow{row})
		gui.Panels.ECS.SetSelectedLineIdx(0)
		return gui.ECSActions()
	})
}

func actionNames(actions []resources.Action) []string {
	names := make([]string, len(actions))
	for i, action := range actions {
		names[i] = action.Name
	}
	return names
}

func findAction(actions []resources.Action, needle string) (resources.Action, bool) {
	for _, action := range actions {
		if strings.Contains(action.Name, needle) {
			return action, true
		}
	}
	return resources.Action{}, false
}

// Action availability must follow the current ECS drill level.
func TestECSActionsDifferPerDrillLevel(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	clusters := ecsAt(t, gui,
		ecsDrillState{level: ecsLevelClusters},
		&ecsRow{Kind: ecsRowKindCluster, Cluster: &aws.ECSCluster{Name: "prod"}})
	if _, ok := findAction(clusters, "Delete cluster"); !ok {
		t.Errorf("cluster actions = %v, want the cluster delete", actionNames(clusters))
	}

	services := ecsAt(t, gui,
		ecsDrillState{level: ecsLevelServices, cluster: "prod"},
		&ecsRow{Kind: ecsRowKindService, Service: &aws.ECSService{Name: "api", DesiredCount: 2}})
	for _, want := range []string{"Scale to N tasks", "Force new deployment", "Enable execute-command", "Delete service"} {
		if _, ok := findAction(services, want); !ok {
			t.Errorf("service actions = %v, want %q among them", actionNames(services), want)
		}
	}
	if _, ok := findAction(services, "Delete cluster"); ok {
		t.Error("the cluster delete is offered on a service row")
	}

	tasks := ecsAt(t, gui,
		ecsDrillState{level: ecsLevelTasks, cluster: "prod", service: "api"},
		&ecsRow{Kind: ecsRowKindTask, Task: &aws.ECSTask{ID: "abc123", Arn: "arn:task/abc123", Containers: []aws.ECSContainer{{Name: "web"}}}})
	for _, want := range []string{"Exec into container", "Stop task"} {
		if _, ok := findAction(tasks, want); !ok {
			t.Errorf("task actions = %v, want %q among them", actionNames(tasks), want)
		}
	}
	if _, ok := findAction(tasks, "Delete service"); ok {
		t.Error("the service delete is offered on a task row")
	}
}

func TestECSDeletesCostTheNameTypedOut(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	cluster, _ := findAction(ecsAt(t, gui,
		ecsDrillState{level: ecsLevelClusters},
		&ecsRow{Kind: ecsRowKindCluster, Cluster: &aws.ECSCluster{Name: "prod"}}), "Delete cluster")
	if cluster.Confirm != resources.ConfirmDangerous || cluster.Token != "prod" {
		t.Errorf("cluster delete: confirm=%v token=%q, want dangerous and the cluster's name", cluster.Confirm, cluster.Token)
	}

	service, _ := findAction(ecsAt(t, gui,
		ecsDrillState{level: ecsLevelServices, cluster: "prod"},
		&ecsRow{Kind: ecsRowKindService, Service: &aws.ECSService{Name: "api"}}), "Delete service")
	if service.Confirm != resources.ConfirmDangerous || service.Token != "api" {
		t.Errorf("service delete: confirm=%v token=%q, want dangerous and the service's name", service.Confirm, service.Token)
	}
}

// Absolute scaling prompts must show the current desired count.
func TestScalePromptShowsTheCurrentCount(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	scale, ok := findAction(ecsAt(t, gui,
		ecsDrillState{level: ecsLevelServices, cluster: "prod"},
		&ecsRow{Kind: ecsRowKindService, Service: &aws.ECSService{Name: "api", DesiredCount: 4}}), "Scale to N")
	if !ok {
		t.Fatal("no scale action")
	}
	if !strings.Contains(scale.Prompt, "api") || !strings.Contains(scale.Prompt, "4") {
		t.Errorf("scale prompt = %q, want the service and the count it is on now", scale.Prompt)
	}
}

// Exec enablement must warn that existing tasks retain their old setting.
func TestTheExecWarningIsOnTheExecToggle(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	toggle, ok := findAction(ecsAt(t, gui,
		ecsDrillState{level: ecsLevelServices, cluster: "prod"},
		&ecsRow{Kind: ecsRowKindService, Service: &aws.ECSService{Name: "api"}}), "execute-command")
	if !ok {
		t.Fatal("no execute-command action")
	}
	if !strings.Contains(toggle.Confirmation, "after") {
		t.Errorf("confirmation = %q, want it to say existing tasks are unaffected", toggle.Confirmation)
	}
}

func TestParseDesiredCount(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    int32
		wantErr bool
	}{
		{in: "3", want: 3},
		{in: "  3  ", want: 3},
		{in: "0", want: 0},
		{in: "-1", wantErr: true},
		{in: "two", wantErr: true},
		{in: "", wantErr: true},
		{in: "3.5", wantErr: true},
		{in: "999999", wantErr: true},
	} {
		got, err := parseDesiredCount(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseDesiredCount(%q) error = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("parseDesiredCount(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// Stop confirmation must disclose service-managed replacement.
func TestStopTaskSaysWhatHappensNext(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	stop, ok := findAction(ecsAt(t, gui,
		ecsDrillState{level: ecsLevelTasks, cluster: "prod", service: "api"},
		&ecsRow{Kind: ecsRowKindTask, Task: &aws.ECSTask{ID: "abc123", Arn: "arn:task/abc123"}}), "Stop task")
	if !ok {
		t.Fatal("no stop action")
	}
	if !strings.Contains(stop.Confirmation, "api") || !strings.Contains(stop.Confirmation, "replacement") {
		t.Errorf("confirmation = %q, want it to name the service that will replace the task", stop.Confirmation)
	}
}
