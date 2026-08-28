package ui

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/config"
	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/tasks"
)

// tabsOf erases the panel's item type so every registry can be checked by one table.
type tabsOf struct {
	name string

	keys   []string
	titles []string

	// render is the first tab's task, built the way the panel builds it, so the table checks the wiring and not a closure the test wrote.
	render func() tasks.TaskFunc
}

func registryOf[T any](name string, state *panels.ContextState[T], item T) tabsOf {
	tabs := state.GetMainTabs()

	keys := make([]string, len(tabs))
	for i, tab := range tabs {
		keys[i] = tab.Key
	}

	return tabsOf{
		name:   name,
		keys:   keys,
		titles: state.GetMainTabTitles(),
		render: func() tasks.TaskFunc { return tabs[0].Render(item) },
	}
}

// overviewFirstConfig turns the auto-refresh off so a tab's task renders once and returns instead of blocking on a ticker.
func overviewFirstConfig() config.UserConfig {
	user := config.DefaultUserConfig()
	user.Refresh.OverviewSeconds = 0

	return user
}

// Overview is the first tab of every resource panel, and it renders. Both halves matter: a registry can list the tab and still hand back a task that draws nothing.
func TestResourcePanelsOpenOnOverview(t *testing.T) {
	gui, g := newHeadlessGuiWithConfig(t, overviewFirstConfig())
	resizeView(t, g, "main", 80, 20)

	gui.ecsDrill.level = ecsLevelClusters
	ecsClusters := registryOf("ecs clusters", gui.Panels.ECS.ContextState, &ecsRow{Kind: ecsRowKindCluster, Cluster: &aws.ECSCluster{Name: "c1"}})
	gui.ecsDrill.level = ecsLevelServices
	ecsServices := registryOf("ecs services", gui.Panels.ECS.ContextState, &ecsRow{Kind: ecsRowKindService, Service: &aws.ECSService{Name: "s1"}})
	gui.ecsDrill.level = ecsLevelClusters

	tests := []struct {
		registry tabsOf
		want     string
	}{
		{registry: registryOf("ec2", gui.Panels.EC2.ContextState, &aws.Instance{ID: "i-1"}), want: "instance overview unavailable"},
		{registry: ecsClusters, want: "cluster overview unavailable"},
		{registry: ecsServices, want: "service overview unavailable"},
		{registry: registryOf("s3", gui.Panels.S3.ContextState, &aws.Bucket{Name: "b1"}), want: "bucket overview unavailable"},
		{registry: registryOf("eks", gui.Panels.EKS.ContextState, &aws.EKSCluster{Name: "k1"}), want: "cluster overview unavailable"},
		{registry: registryOf("ecr", gui.Panels.ECR.ContextState, &aws.ECRRepository{Name: "r1"}), want: "repository overview unavailable"},
		{registry: registryOf("secrets", gui.Panels.Secrets.ContextState, &aws.SecretSummary{Name: "s1"}), want: "secret overview unavailable"},
		{registry: registryOf("vpc", gui.Panels.VPC.ContextState, &aws.VPC{ID: "vpc-1"}), want: "VPC overview unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.registry.name, func(t *testing.T) {
			if got := tt.registry.keys[0]; got != overviewTabKey {
				t.Errorf("first tab key = %q, want %q (tabs: %v)", got, overviewTabKey, tt.registry.keys)
			}
			if got := tt.registry.titles[0]; got != "Overview" {
				t.Errorf("first tab title = %q, want %q", got, "Overview")
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			task := ask(g, tt.registry.render)
			task(ctx)

			if got := mainBufferWithin(g, gui, tt.want, time.Second); !strings.Contains(got, tt.want) {
				t.Errorf("main = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

// A profile is not an AWS resource and an ECS task's detail is its task definition, so neither gets an Overview tab: an empty first tab that nothing will ever fill is worse than no tab.
func TestPanelsWithoutAResourceToOverviewHaveNoOverviewTab(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	gui.ecsDrill.level = ecsLevelTasks
	ecsTasks := registryOf("ecs tasks", gui.Panels.ECS.ContextState, &ecsRow{Kind: ecsRowKindTask})
	gui.ecsDrill.level = ecsLevelClusters

	for _, registry := range []tabsOf{
		registryOf("profile", gui.Panels.Profile.ContextState, "default"),
		ecsTasks,
	} {
		if slices.Contains(registry.keys, overviewTabKey) {
			t.Errorf("%s tabs = %v, want no %q tab", registry.name, registry.keys, overviewTabKey)
		}
	}
}

// Cycling has to wrap through Overview like any other tab, or the tab it fronts becomes unreachable with `[`.
func TestOverviewIsPartOfTheTabCycle(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	state := gui.Panels.EC2.ContextState
	last := state.GetMainTabs()[len(state.GetMainTabs())-1].Key

	if got := state.GetCurrentMainTab().Key; got != overviewTabKey {
		t.Fatalf("initial tab = %q, want %q", got, overviewTabKey)
	}

	// EC2 is down to the one Overview tab, so cycling in either direction must land back on it rather than walking off the registry.
	state.HandleNextMainTab()
	if got := state.GetCurrentMainTab().Key; got != overviewTabKey {
		t.Errorf("next from overview = %q, want %q", got, overviewTabKey)
	}

	state.HandlePrevMainTab()
	state.HandlePrevMainTab()
	if got := state.GetCurrentMainTab().Key; got != last {
		t.Errorf("prev from overview = %q, want the last tab %q", got, last)
	}
}

// A profile switch resets the ECS drill level without touching the tab index, so the services level's last tab has to survive landing on the shorter cluster-level set.
// Prepending Overview widened the gap by one tab, which is why it is pinned here.
func TestECSTabIndexSurvivesAProfileSwitch(t *testing.T) {
	gui := newTestGui(t)
	state := gui.Panels.ECS.ContextState

	gui.ecsDrill.level = ecsLevelServices
	state.SetMainTabIndex(len(state.GetMainTabs()) - 1)

	gui.resetDependentPanelState()

	if got := state.GetCurrentMainTab().Key; got != overviewTabKey {
		t.Errorf("current tab = %q, want %q after the drill level reset", got, overviewTabKey)
	}
}

// The header must agree with the tab being rendered. renderContext reads mainTabIdx straight into main's TabIndex, so this is the user-visible end of the same clamp: without it the read is what panics.
func TestMainTabHeaderCannotPointPastTheTabsItShows(t *testing.T) {
	gui, g := newHeadlessGuiWithConfig(t, overviewFirstConfig())
	resizeView(t, g, "main", 80, 20)

	gui.ecsDrill.level = ecsLevelServices
	gui.Panels.ECS.ContextState.SetMainTabIndex(len(gui.Panels.ECS.ContextState.GetMainTabs()) - 1)

	gui.ecsDrill = ecsDrillState{}
	gui.Panels.ECS.SetItems([]*ecsRow{{Kind: ecsRowKindCluster, Cluster: &aws.ECSCluster{Name: "c1"}}})

	run(t, g, gui.Panels.ECS.HandleSelect)

	tabs := ask(g, func() []string { return gui.Views.Main.Tabs })
	index := ask(g, func() int { return gui.Views.Main.TabIndex })
	if index < 0 || index >= len(tabs) {
		t.Fatalf("main TabIndex = %d with %d tabs %v, want an index that exists", index, len(tabs), tabs)
	}
	if tabs[index] != "Overview" {
		t.Errorf("main highlights %q, want the %q tab that was actually rendered", tabs[index], "Overview")
	}
}

func TestOverviewUnavailableNamesTheResourceAndIsMuted(t *testing.T) {
	forceColor(t)

	got := overviewUnavailable("bucket")
	if !strings.Contains(got, "bucket overview unavailable") {
		t.Errorf("overviewUnavailable(%q) = %q, want it to name the resource", "bucket", got)
	}
	// color.Faint is attribute 2; the escape is what tells a muted line from a plain one, and gocui strips it out of View.Buffer().
	if !strings.HasPrefix(got, "\x1b[2m") {
		t.Errorf("overviewUnavailable(%q) = %q, want it wrapped in the faint attribute", "bucket", got)
	}
}
