package ui

import (
	"strings"
	"testing"

	"github.com/jesseduffield/gocui"

	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/ui/utils"
)

// The options line is the only place a panel's own keys are advertised on screen, so what it says has to depend on which panel is focused.
func TestDashboardOptionsAreContextualPerPanel(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	cases := []struct {
		view   string
		wants  []string
		absent []string
	}{
		{view: "ec2", wants: []string{"enter inspect", "c connect"}, absent: []string{"e exec", "v reveal", "[ ] tabs"}},
		{view: "ecs", wants: []string{"enter drill down", "e exec"}, absent: []string{"c connect", "enter inspect"}},
		{view: "secrets", wants: []string{"enter inspect", "v reveal"}, absent: []string{"c connect", "e exec"}},
		{view: "profile", wants: []string{"enter switch"}, absent: []string{"enter inspect", "c connect"}},
		{view: "s3", wants: []string{"enter inspect", "/ filter"}, absent: []string{"c connect", "e exec", "v reveal"}},
		{view: "main", wants: []string{"[ ] tabs", "enter select", "←→↑↓ scroll"}, absent: []string{"/ filter", "navigate"}},
	}

	for _, tc := range cases {
		t.Run(tc.view, func(t *testing.T) {
			line := optionsToString(gui.dashboardOptions(tc.view))

			for _, want := range tc.wants {
				if !strings.Contains(line, want) {
					t.Errorf("the %s options line does not offer %q: %s", tc.view, want, line)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(line, absent) {
					t.Errorf("the %s options line offers %q, which does not work there: %s", tc.view, absent, line)
				}
			}
		})
	}
}

// The copy key is new and invisible unless the footer says so, and the vocabulary is the redesign's, not each panel's own wording.
func TestEveryResourceViewAdvertisesTheSharedVocabulary(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	for _, name := range resourceViewNames(gui.allSidePanels()) {
		line := optionsToString(gui.dashboardOptions(name))
		for _, want := range []string{"y copy", "r refresh", "a actions", "q quit"} {
			if !strings.Contains(line, want) {
				t.Errorf("the %s options line does not offer %q: %s", name, want, line)
			}
		}
	}
}

// The line reads in frequency order, which is the whole reason it is a slice and not the alphabetical map the popups use: sorted by keycap, "/ filter" leads and "navigate" lands in the middle.
// Asserted on the RENDERED line rather than on the slice handed to the renderer: with the order checked one step early, a renderer that sorts its parts keeps every assertion green and reorders the footer anyway.
func TestDashboardOptionsReadInReadingOrder(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	line := optionsToString(gui.dashboardOptions("ec2"))

	want := []string{"navigate", "enter inspect", "y copy", "r refresh", "/ filter", "a actions", "c connect", "q quit"}
	at := make([]int, len(want))
	for i, entry := range want {
		at[i] = strings.Index(line, entry)
		if at[i] == -1 {
			t.Fatalf("the ec2 options line is missing %q: %s", entry, line)
		}
		if i > 0 && at[i] < at[i-1] {
			t.Errorf("%q reads before %q, which is not the vocabulary's order: %s", entry, want[i-1], line)
		}
	}

	if !strings.HasPrefix(line, "←→↑↓ navigate") {
		t.Errorf("the line does not open on navigate: %s", line)
	}
	if !strings.HasSuffix(line, "q quit") {
		t.Errorf("the line does not end on quit, so quit is not the first entry a narrow terminal cuts: %s", line)
	}
}

// The filter label and the filter BINDING answer to the same condition, and every shipped list is filterable, so this is the only way to drive the other side of it: without this the footer could advertise a key that does nothing on a future unfilterable panel.
func TestAnUnfilterablePanelDoesNotAdvertiseFilter(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	if line := optionsToString(gui.dashboardOptions("s3")); !strings.Contains(line, "filter") {
		t.Fatalf("the s3 list is filterable but its footer does not say so, so this test cannot see the guard: %s", line)
	}

	gui.Panels.S3.DisableFilter = true

	if line := optionsToString(gui.dashboardOptions("s3")); strings.Contains(line, "filter") {
		t.Errorf("a panel with filtering disabled still advertises it: %s", line)
	}
}

// A label naming a key the user has rebound is worse than no label, which is why nothing here hardcodes a keycap that config can move.
func TestDashboardOptionsFollowARebind(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	keymap, problems := buildKeymap(map[string]string{"copy-id": "Y", "filter": "f"})
	if len(problems) > 0 {
		t.Fatalf("rebinding copy-id and filter reported problems: %v", problems)
	}
	gui.Keys = keymap

	line := optionsToString(gui.dashboardOptions("ec2"))
	if !strings.Contains(line, "Y copy") {
		t.Errorf("the footer ignores the rebound copy key: %s", line)
	}
	if !strings.Contains(line, "f filter") {
		t.Errorf("the footer ignores the rebound filter key: %s", line)
	}
	if strings.Contains(line, "y copy") || strings.Contains(line, "/ filter") {
		t.Errorf("the footer still shows a default keycap the user moved: %s", line)
	}
}

// The contextual line is worth nothing if the render call does not pass the view the user is actually on, and that argument is a call site no test of dashboardOptions can see: pinned to one view name, every assertion above stays green while the footer says the same thing everywhere.
// Driven through renderPanelOptions rather than renderGlobalOptions, so the dispatch that chooses the dashboard line over a popup's is covered by the same test.
func TestTheFooterRendersTheFocusedPanelsLine(t *testing.T) {
	gui, g := newHeadlessGui(t)

	for _, tc := range []struct {
		view *gocui.View
		want string
	}{
		{view: gui.Views.EC2, want: "c connect"},
		{view: gui.Views.Secrets, want: "v reveal"},
		{view: gui.Views.ECS, want: "e exec"},
	} {
		run(t, g, func() error { return gui.switchFocus(tc.view) })
		run(t, g, gui.renderPanelOptions)

		if line := waitForView(t, g, gui.Views.Options, tc.want); !strings.Contains(line, tc.want) {
			t.Errorf("with %s focused the footer reads %q, want it to offer %q", tc.view.Name(), line, tc.want)
		}
	}
}

// The footer shares one terminal row with the app status and the version, and gocui cuts what does not fit rather than wrapping.
// This is a budget, not an exact-width assertion: it fails when a future entry pushes the longest line past what a normal terminal shows, which is the point at which items start disappearing silently.
func TestNoDashboardOptionsLineOutgrowsTheFooter(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	const budget = 90

	for _, name := range resourceViewNames(gui.allSidePanels()) {
		line := optionsToString(gui.dashboardOptions(name))
		if width := runewidth.StringWidth(utils.Decolorise(line)); width > budget {
			t.Errorf("the %s options line is %d cells, over the %d-cell budget: %s", name, width, budget, line)
		}
	}
}
