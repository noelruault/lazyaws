package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/noelruault/lazyaws/apps/aws"
)

// The registry has to hand back the real overview rather than the placeholder it replaced, and the nil-client guard cannot show that: it renders the same "service overview unavailable" the placeholder did.
// A client whose SDK clients are all nil is what tells them apart — every fetch fails without a network call, and the pane still has to render everything DescribeServices already answered.
func TestECSServicePanelRendersTheServiceOverview(t *testing.T) {
	gui, g := newHeadlessGuiWithConfig(t, overviewFirstConfig())
	resizeView(t, g, "main", 80, 24)
	gui.Client = &aws.Client{}

	gui.ecsDrill.level = ecsLevelServices
	registry := registryOf("ecs services", gui.Panels.ECS.ContextState, &ecsRow{
		Kind:    ecsRowKindService,
		Service: &aws.ECSService{Name: "app-auth", Cluster: "app-cluster", DesiredCount: 2, RunningCount: 1},
	})
	gui.ecsDrill.level = ecsLevelClusters

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := ask(g, registry.render)
	task(ctx)

	got := mainBufferWithin(g, gui, "Deployment", time.Second)
	for _, want := range []string{"app-auth", "2 desired / 1 running / 0 pending", "Deployment", "Networking", "Recent events"} {
		if !strings.Contains(got, want) {
			t.Errorf("main = %q, want it to contain %q", got, want)
		}
	}
}

// The service overview tab is built for one drill level and rendered with whatever row the panel holds when the task runs, which a rerender queued across a drill can make the wrong kind.
// The formatter reads the service unguarded, so without the row check this is a panic that takes the app down rather than a tab that says nothing.
func TestECSServiceOverviewToleratesARowWithoutAService(t *testing.T) {
	gui, g := newHeadlessGuiWithConfig(t, overviewFirstConfig())
	resizeView(t, g, "main", 80, 24)
	gui.Client = &aws.Client{}

	got := ask(g, func() string {
		return gui.ecsServiceOverview(context.Background(), &ecsRow{Kind: ecsRowKindService}, 80)
	})

	if !strings.Contains(got, "service overview unavailable") {
		t.Errorf("ecsServiceOverview() = %q, want the unavailable statement rather than a panic", got)
	}
}
