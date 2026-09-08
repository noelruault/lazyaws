package ui

import (
	"fmt"
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

	// WAITED, not read once: RerenderList paints through gocui's Update queue, so reading the buffer straight after it passes locally and fails on a slower runner, which is exactly how this test first broke CI.
	// The starting state is the bug, so proving the fix means proving a narrow render still produces it.
	waitForView(t, g, gui.Views.Profile, "alpha-p…")

	run(t, g, func() error {
		_, err := g.SetView(gui.Views.Profile.Name(), 0, 0, 60, 20, 0)
		return err
	})
	run(t, g, func() error { return gui.layout(g) })

	after := waitForView(t, g, gui.Views.Profile, "alpha-production")
	for _, want := range []string{"alpha-production", "bravo-datalake", "charlie-security"} {
		if !strings.Contains(after, want) {
			t.Errorf("after the view was widened the list still hides %q:\n%s", want, after)
		}
	}
}

// A settled layout must not re-render: this runs on every frame, and eight panels re-rendering per frame would burn the terminal for nothing.
// Detected with a sentinel rather than by comparing buffers, because a re-render reproduces the same text: only something the render would destroy can tell the two apart.
func TestASettledLayoutDoesNotReRenderTheList(t *testing.T) {
	gui, g := newHeadlessGui(t)

	gui.Panels.Profile.SetItems([]string{"alpha-production"})
	run(t, g, func() error { return gui.layout(g) })
	run(t, g, gui.Panels.Profile.RerenderList)
	waitForView(t, g, gui.Views.Profile, "alpha-production")

	const sentinel = "SENTINEL-NOT-A-RENDER"
	run(t, g, func() error {
		gui.Views.Profile.Clear()
		fmt.Fprint(gui.Views.Profile, sentinel)

		return nil
	})

	// Nothing changed the width, so this pass has to leave the view alone; a re-render clears it and the sentinel goes with it.
	run(t, g, func() error { return gui.layout(g) })
	if got := readView(g, gui.Views.Profile); !strings.Contains(got, sentinel) {
		t.Errorf("a layout pass that changed no width re-rendered the list anyway; view now reads %q", got)
	}
}
