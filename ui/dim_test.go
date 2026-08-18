package ui

import (
	"testing"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/config"
)

// The headless harness has no layout pass, so tests call syncDim directly.
func syncDim(t *testing.T, g *gocui.Gui, gui *Gui) {
	t.Helper()
	run(t, g, func() error { gui.syncDim(); return nil })
}

func foregrounds(g *gocui.Gui) map[string]gocui.Attribute {
	return ask(g, func() map[string]gocui.Attribute {
		out := map[string]gocui.Attribute{}
		for _, view := range g.Views() {
			out[view.Name()] = view.FgColor
		}
		return out
	})
}

func dashboardIsDimmed(g *gocui.Gui, gui *Gui) bool {
	return ask(g, func() bool { return gui.Views.EC2.FgColor&gocui.AttrDim != 0 })
}

// Dimming must restore the actual theme color, not an assumed default.
func TestDimIsAnExactRoundTrip(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, func() error {
		gui.Views.EC2.FgColor = gocui.ColorCyan | gocui.AttrBold
		return nil
	})

	before := foregrounds(g)

	run(t, g, func() error { gui.Views.Menu.Visible = true; return nil })
	syncDim(t, g, gui)

	for name, fg := range foregrounds(g) {
		if gui.isPopupPanel(name) {
			if fg != before[name] {
				t.Errorf("the popup view %q was dimmed along with the background", name)
			}
			continue
		}
		if fg&gocui.AttrDim == 0 {
			t.Errorf("view %q was not dimmed", name)
		}
	}

	run(t, g, func() error { gui.Views.Menu.Visible = false; return nil })
	syncDim(t, g, gui)

	for name, fg := range foregrounds(g) {
		if fg != before[name] {
			t.Errorf("view %q came back as %v, want %v", name, fg, before[name])
		}
	}
	if n := ask(g, func() int { return len(gui.dimmed) }); n != 0 {
		t.Errorf("%d views are still recorded as dimmed after closing", n)
	}
}

// Repeated layout passes must not save an already-dimmed color as the original.
func TestDimIsIdempotent(t *testing.T) {
	gui, g := newHeadlessGui(t)

	before := ask(g, func() gocui.Attribute { return gui.Views.EC2.FgColor })

	run(t, g, func() error { gui.Views.Menu.Visible = true; return nil })
	for range 3 {
		syncDim(t, g, gui)
	}

	run(t, g, func() error { gui.Views.Menu.Visible = false; return nil })
	syncDim(t, g, gui)

	if got := ask(g, func() gocui.Attribute { return gui.Views.EC2.FgColor }); got != before {
		t.Fatalf("after three dim passes the foreground came back as %v, want %v", got, before)
	}
}

func TestDimCanBeTurnedOff(t *testing.T) {
	user := config.DefaultUserConfig()
	user.Gui.DimBehindPopups = false
	gui, g := newHeadlessGuiWithConfig(t, user)

	before := ask(g, func() gocui.Attribute { return gui.Views.EC2.FgColor })

	run(t, g, func() error { gui.Views.Menu.Visible = true; return nil })
	syncDim(t, g, gui)

	if got := ask(g, func() gocui.Attribute { return gui.Views.EC2.FgColor }); got != before {
		t.Fatalf("dimBehindPopups is off but %v became %v", before, got)
	}
}

func TestTurningDimOffGivesTheColoursBack(t *testing.T) {
	gui, g := newHeadlessGui(t)

	before := foregrounds(g)

	run(t, g, func() error { gui.Views.Menu.Visible = true; return nil })
	syncDim(t, g, gui)
	if !dashboardIsDimmed(g, gui) {
		t.Fatal("the dashboard did not dim in the first place")
	}

	run(t, g, func() error { gui.Config.User.Gui.DimBehindPopups = false; return nil })
	syncDim(t, g, gui)

	for name, fg := range foregrounds(g) {
		if fg != before[name] {
			t.Errorf("turning the setting off left %q at %v, want %v", name, fg, before[name])
		}
	}
}

// Real popup paths keep new popup types covered by the dimming invariant.
func TestPopupsDimAndUndim(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, func() error {
		return gui.Menu(CreateMenuOptions{Title: "Actions", Items: gui.actionMenuItems(newSpy("Do a thing").listOfOne())})
	})
	syncDim(t, g, gui)
	if !dashboardIsDimmed(g, gui) {
		t.Error("opening the menu did not dim the dashboard")
	}

	run(t, g, gui.handleMenuClose)
	syncDim(t, g, gui)
	if dashboardIsDimmed(g, gui) {
		t.Error("closing the menu left the dashboard dimmed")
	}

	// Confirmation creation is queued, so the assertion must wait for visibility.
	run(t, g, func() error { return gui.createConfirmationPanel("Sure?", "really?", nil, nil) })
	waitFor(t, g, func() bool { return gui.Views.Confirmation.Visible }, "the confirmation popup to appear")
	syncDim(t, g, gui)
	if !dashboardIsDimmed(g, gui) {
		t.Error("opening a confirmation did not dim the dashboard")
	}

	run(t, g, gui.closeConfirmationPrompt)
	syncDim(t, g, gui)
	if dashboardIsDimmed(g, gui) {
		t.Error("closing a confirmation left the dashboard dimmed")
	}

	// Screen swaps hide popups without changing focus, so layout must restore colors.
	run(t, g, func() error { return gui.createConfirmationPanel("Sure?", "really?", nil, nil) })
	waitFor(t, g, func() bool { return gui.Views.Confirmation.Visible }, "the confirmation popup to appear")
	run(t, g, func() error { gui.dismissPopups(); return nil })
	syncDim(t, g, gui)
	if dashboardIsDimmed(g, gui) {
		t.Error("dismissing popups on a screen swap left the dashboard dimmed")
	}
}
