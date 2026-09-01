package ui

import (
	"testing"
)

// Detail-tab bindings must remain active after focus enters the main view.
// The keys are read from the keymap rather than written out here, so a preset or a rebind that moves them cannot fail a test that is really about the main view keeping them.
func TestMainViewHasTabKeybindings(t *testing.T) {
	gui := newTestGui(t)

	bound := map[KeyName]bool{}
	for _, b := range gui.GetInitialKeybindings() {
		if b.ViewName != "main" {
			continue
		}
		for _, name := range []KeyName{KeyPrevTab, KeyNextTab} {
			if b.Key == gui.Keys.Get(name).Key {
				bound[name] = true
			}
		}
	}

	for _, name := range []KeyName{KeyPrevTab, KeyNextTab} {
		if !bound[name] {
			t.Errorf("%q (%v) is not bound on the \"main\" view", name, gui.Keys.Get(name).Key)
		}
	}
}

func TestECSServiceTabsCycle(t *testing.T) {
	gui := newTestGui(t)
	gui.ecsDrill.level = ecsLevelServices

	ctx := gui.Panels.ECS.ContextState

	want := []string{"overview", "config", "deployments", "events", "taskdef", "overview"}
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
