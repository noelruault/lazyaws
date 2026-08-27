package ui

import (
	"testing"
)

// Detail-tab bindings must remain active after focus enters the main view.
func TestMainViewHasTabKeybindings(t *testing.T) {
	gui := newTestGui(t)

	var hasPrev, hasNext bool
	for _, b := range gui.GetInitialKeybindings() {
		if b.ViewName != "main" {
			continue
		}
		switch b.Key {
		case '[':
			hasPrev = true
		case ']':
			hasNext = true
		}
	}

	if !hasPrev {
		t.Error(`'[' (previous tab) is not bound on the "main" view`)
	}
	if !hasNext {
		t.Error(`']' (next tab) is not bound on the "main" view`)
	}
}

func TestECSServiceTabsCycle(t *testing.T) {
	gui := newTestGui(t)
	gui.ecsDrill.level = ecsLevelServices

	ctx := gui.Panels.ECS.ContextState

	want := []string{"overview", "config", "deployments", "events", "scaling", "taskdef", "overview"}
	if got := ctx.GetCurrentMainTab().Key; got != want[0] {
		t.Fatalf("initial tab = %q, want %q", got, want[0])
	}
	for i := 1; i < len(want); i++ {
		ctx.HandleNextMainTab()
		if got := ctx.GetCurrentMainTab().Key; got != want[i] {
			t.Fatalf("after %d next: tab = %q, want %q", i, got, want[i])
		}
	}

	ctx.HandlePrevMainTab()
	if got := ctx.GetCurrentMainTab().Key; got != "taskdef" {
		t.Fatalf("prev from overview = %q, want %q", got, "taskdef")
	}
}

// Main-view tab actions must resolve the side panel beneath it in the focus stack.
func TestSidePanelForMainResolvesBackingPanel(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	gui.State.ViewStack = []string{"ecs", "main"}

	panel, ok := gui.sidePanelForMain()
	if !ok {
		t.Fatal("sidePanelForMain returned ok=false")
	}
	if got := panel.GetView().Name(); got != "ecs" {
		t.Fatalf("sidePanelForMain resolved to %q, want %q", got, "ecs")
	}
}
