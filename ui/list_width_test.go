package ui

import (
	"strings"
	"testing"
)

// Views are created 10 columns wide and the first render can land before the first layout pass, so a list laid out then keeps 8-character rows: a fifteen-character profile name renders as eight characters and an ellipsis.
// A refresh tick normally re-renders and hides it, which is why it showed up only when signed out, where the tick returns before rendering. So the layout pass has to be what fixes it.
func TestASideListReLaysOutWhenItsViewIsResized(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, func() error {
		_, err := g.SetView(gui.Views.Profile.Name(), 0, 0, 10, 10, 0)
		return err
	})
	gui.Panels.Profile.SetItems([]string{"alpha-production", "bravo-datalake", "charlie-security"})
	run(t, g, gui.Panels.Profile.RerenderList)

	// The starting state is the bug: proving the fix means proving this is what a narrow render produces.
	if narrow := ask(g, gui.Views.Profile.Buffer); !strings.Contains(narrow, "alpha-p…") {
		t.Fatalf("a 10-column view did not truncate, so this test no longer reproduces anything:\n%s", narrow)
	}

	run(t, g, func() error {
		_, err := g.SetView(gui.Views.Profile.Name(), 0, 0, 60, 20, 0)
		return err
	})
	run(t, g, func() error { return gui.layout(g) })

	after := ask(g, gui.Views.Profile.Buffer)
	for _, want := range []string{"alpha-production", "bravo-datalake", "charlie-security"} {
		if !strings.Contains(after, want) {
			t.Errorf("after the view was widened the list still hides %q:\n%s", want, after)
		}
	}
}

// A settled layout must not re-render: this runs on every frame, and eight panels re-rendering per frame would burn the terminal for nothing.
func TestASettledLayoutDoesNotReRenderTheList(t *testing.T) {
	gui, g := newHeadlessGui(t)

	gui.Panels.Profile.SetItems([]string{"alpha-production"})
	run(t, g, func() error { return gui.layout(g) })
	run(t, g, gui.Panels.Profile.RerenderList)
	before := ask(g, gui.Views.Profile.Buffer)

	// Nothing changed the width, so the second pass has to be a no-op rather than a re-render.
	run(t, g, func() error { return gui.layout(g) })
	if after := ask(g, gui.Views.Profile.Buffer); after != before {
		t.Errorf("a layout pass that changed no width still re-rendered:\nbefore %q\nafter  %q", before, after)
	}
}
