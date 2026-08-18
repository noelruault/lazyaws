package ui

import (
	"errors"
	"testing"

	"github.com/noelruault/lazyaws/ui/resources"
)

// Focus assertions must run on the gocui loop to avoid racing view writes.

func goTo(t *testing.T, gui *Gui, input string) error {
	t.Helper()

	ref, err := gui.Registry.Resolve(input)
	if err != nil {
		t.Fatalf("Resolve(%q): %v", input, err)
	}

	return ask(gui.g, func() error { return gui.Registry.FocusRef(ref) })
}

// Every registered alias must resolve to an existing view.
func TestRegistryResolvesEveryPanel(t *testing.T) {
	gui, g := newHeadlessGui(t)

	for _, tc := range []struct {
		input string
		view  string
	}{
		{":profiles", "profile"},
		{":ecs", "ecs"},
		{":ec2", "ec2"},
		{":s3", "s3"},
		{":eks", "eks"},
		{":ecr", "ecr"},
		{":secrets", "secrets"},
		{":aws:ecs:clusters", "ecs"},
		{":aws:s3:buckets", "s3"},
		{":scrts", "secrets"},
		{": ecr", "ecr"},
	} {
		if err := goTo(t, gui, tc.input); err != nil {
			t.Errorf("%q: %v", tc.input, err)
			continue
		}
		if got := focusedView(g, gui); got != tc.view {
			t.Errorf("%q focused %q, want %q", tc.input, got, tc.view)
		}
	}
}

// Every side panel must remain addressable through the command bar.
func TestRegistryCoversEverySidePanel(t *testing.T) {
	gui, g := newHeadlessGui(t)

	focusable := map[string]bool{}
	for _, entry := range gui.Registry.Entries() {
		if entry.Focus == nil {
			continue
		}
		if err := ask(g, func() error { return entry.Focus(entry.Ref) }); err != nil {
			t.Errorf("focusing %s: %v", entry.Ref, err)
			continue
		}
		focusable[focusedView(g, gui)] = true
	}

	for _, panel := range gui.allSidePanels() {
		if name := panel.GetView().Name(); !focusable[name] {
			t.Errorf("no registry entry focuses the %q panel", name)
		}
	}
}

// Panel-to-resource mappings must stay synchronized with provider registration.
func TestPanelRefsMatchTheRegistry(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	panels := map[string]bool{}
	for name, key := range panelRefs() {
		entry, ok := gui.Registry.Get(key)
		if !ok {
			t.Errorf("panelRefs[%q] points at %s, which is not registered", name, key.Name())
			continue
		}
		if entry.Actions == nil {
			t.Errorf("panelRefs[%q] points at %s, which has no actions, so the actions key does nothing there", name, key.Name())
		}
		panels[name] = true
	}

	for _, panel := range gui.allSidePanels() {
		if name := panel.GetView().Name(); !panels[name] {
			t.Errorf("the %q panel is not in panelRefs, so its actions key resolves to nothing", name)
		}
	}
}

// Missing async-loaded selectors must report failure instead of leaving a stale cursor.
func TestFocusSelectsNothingItCannotFind(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	if err := goTo(t, gui, ":ec2:no-such-box"); err == nil {
		t.Fatal("focusing a selector that matches nothing should report it")
	}
}

// Profile selection must use raw identity because the active row is decorated.
func TestFocusFindsTheProfileYouAreOn(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, func() error {
		gui.CurrentProfile = "staging"
		gui.Panels.Profile.SetItems([]string{"default", "staging"})
		return nil
	})

	if err := goTo(t, gui, ":profiles:staging"); err != nil {
		t.Fatalf("going to the profile already in use: %v", err)
	}

	selected := ask(g, func() string {
		profile, err := gui.Panels.Profile.GetSelectedItem()
		if err != nil {
			return ""
		}
		return profile
	})
	if selected != "staging" {
		t.Errorf("cursor landed on %q, want staging", selected)
	}

	if err := goTo(t, gui, ":profiles:no-such-profile"); err == nil {
		t.Error("a profile that is not in the list should be reported, not silently ignored")
	}
}

func TestFocusECSJumpsStraightToTheDrillLevel(t *testing.T) {
	gui, g := newHeadlessGui(t)

	if err := goTo(t, gui, ":ecs:web-cluster:web"); err != nil {
		t.Fatalf("FocusRef: %v", err)
	}

	drill := ask(g, func() ecsDrillState { return gui.ecsDrill })
	if drill.level != ecsLevelTasks {
		t.Errorf("drill level = %v, want tasks", drill.level)
	}
	if drill.cluster != "web-cluster" || drill.service != "web" {
		t.Errorf("drill = %+v, want cluster web-cluster and service web", drill)
	}
	if got := ask(g, func() string { return gui.Views.ECS.Title }); got != "ECS: web-cluster/web" {
		t.Errorf("panel title = %q", got)
	}
}

func TestFocusECSRejectsTooDeepARef(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	if err := goTo(t, gui, ":ecs:a:b:c"); err == nil {
		t.Fatal("an ECS ref three levels deep should be refused, not silently truncated")
	}
}

func TestSettingsIsAddressable(t *testing.T) {
	gui, g := newHeadlessGui(t)

	if err := goTo(t, gui, ":settings"); err != nil {
		t.Fatalf("FocusRef: %v", err)
	}
	if !ask(g, func() bool { return gui.State.Settings.active }) {
		t.Fatal(":settings did not open the settings screen")
	}

	if err := goTo(t, gui, ":ec2"); err != nil {
		t.Fatalf("FocusRef(:ec2): %v", err)
	}
	if ask(g, func() bool { return gui.State.Settings.active }) {
		t.Fatal("jumping to a panel left the settings screen up")
	}
}

func TestUnknownRefIsAnError(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	// Resolve falls back to a fuzzy subsequence match, so this input has to be one no registered name can absorb; anything plausible resolves to its nearest neighbour instead of failing.
	const unknown = ":aws:zzzqqq"

	if _, err := gui.Registry.Resolve(unknown); !errors.Is(err, resources.ErrUnknown) && !errors.Is(err, resources.ErrAmbiguous) {
		t.Fatalf("Resolve(%s) = %v, want a resolution failure", unknown, err)
	}
}
