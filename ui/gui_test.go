package ui

import (
	"testing"

	"github.com/noelruault/lazyaws/config"
)

func TestGuiHeadlessSmoke(t *testing.T) {
	gui, g := newHeadlessGui(t)

	sidePanelNames := []string{"profile", "ecs", "ec2", "s3", "eks", "ecr", "secrets"}
	for _, name := range sidePanelNames {
		view, err := g.View(name)
		if err != nil {
			t.Errorf("View %q does not exist: %v", name, err)
		}
		if view == nil {
			t.Errorf("View %q is nil", name)
		}
	}

	allSidePanels := gui.allSidePanels()
	if len(allSidePanels) != 8 {
		t.Errorf("expected 8 side panels, got %d", len(allSidePanels))
	}

	expectedNames := map[string]bool{
		"profile": true,
		"ecs":     true,
		"ec2":     true,
		"s3":      true,
		"eks":     true,
		"ecr":     true,
		"secrets": true,
		"vpc":     true,
	}

	for _, panel := range allSidePanels {
		name := panel.GetView().Name()
		if !expectedNames[name] {
			t.Errorf("unexpected side panel name: %q", name)
		}
		delete(expectedNames, name)
	}

	if len(expectedNames) > 0 {
		t.Errorf("missing side panels: %v", expectedNames)
	}
}

// Every focused-panel reloader needs its own throttle or the refresh key silently no-ops.
func TestNewGuiPanelThrottlesCoverAllReloaders(t *testing.T) {
	cfg := &config.Config{User: config.UserConfig{Gui: config.GuiConfig{}}}

	gui, err := NewGui(cfg, nil, make(chan error, 1))
	if err != nil {
		t.Fatalf("NewGui failed: %v", err)
	}

	for name := range gui.panelReloaders() {
		if gui.panelThrottles[name] == nil {
			t.Errorf("panelThrottles missing entry for panel %q", name)
		}
	}
	if len(gui.panelThrottles) != len(gui.panelReloaders()) {
		t.Errorf("panelThrottles has %d entries, want %d (one per panelReloaders key)", len(gui.panelThrottles), len(gui.panelReloaders()))
	}
}
