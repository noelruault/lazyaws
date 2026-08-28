package ui

import (
	"strings"
	"testing"

	"github.com/jesseduffield/gocui"

	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/ui/utils"
)

// renderedOptions is the footer line as text: the keycaps carry an ANSI colour when a terminal is attached, and these tests assert vocabulary, not styling.
func renderedOptions(gui *Gui, viewName string) string {
	return utils.Decolorise(optionsToString(gui.dashboardOptions(viewName)))
}

// The options line is the only place a panel's own keys are advertised on screen, so what it says has to depend on which panel is focused.
func TestDashboardOptionsAreContextualPerPanel(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	cases := []struct {
		view   string
		wants  []string
		absent []string
	}{
		{view: "ec2", wants: []string{"Enter Inspect", "c Connect"}, absent: []string{"e Exec", "v Reveal", "[ ] Tabs"}},
		{view: "ecs", wants: []string{"Enter Drill down", "e Exec"}, absent: []string{"c Connect", "Enter Inspect"}},
		{view: "secrets", wants: []string{"Enter Inspect", "v Reveal"}, absent: []string{"c Connect", "e Exec"}},
		{view: "profile", wants: []string{"Enter Switch"}, absent: []string{"Enter Inspect", "c Connect"}},
		{view: "s3", wants: []string{"Enter Inspect", "/ Filter"}, absent: []string{"c Connect", "e Exec", "v Reveal"}},
		{view: "main", wants: []string{"[ ] Tabs", "Enter Select", "Arrows Scroll"}, absent: []string{"/ Filter", "Navigate"}},
	}

	for _, tc := range cases {
		t.Run(tc.view, func(t *testing.T) {
			line := renderedOptions(gui, tc.view)

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

	// The view list is built here, not read back from resourceViewNames: taking the expectation from the function that decides the answer cannot see that function losing a view.
	for _, name := range append(sidePanelViewNames(gui.allSidePanels()), "main") {
		line := renderedOptions(gui, name)
		for _, want := range []string{"y Copy ARN", "r Refresh", "a Actions", "q Quit"} {
			if !strings.Contains(line, want) {
				t.Errorf("the %s options line does not offer %q: %s", name, want, line)
			}
		}
	}
}

// The line reads in frequency order, which is the whole reason it is a slice and not the alphabetical map the popups use: sorted by keycap, "/ Filter" leads and "Navigate" lands in the middle.
// Asserted on the RENDERED line rather than on the slice handed to the renderer: with the order checked one step early, a renderer that sorts its parts keeps every assertion green and reorders the footer anyway.
func TestDashboardOptionsReadInReadingOrder(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	line := renderedOptions(gui, "ec2")

	want := []string{"Navigate", "Enter Inspect", "y Copy ARN", "r Refresh", "/ Filter", "a Actions", "c Connect", "q Quit"}
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

	if !strings.HasPrefix(line, "Arrows Navigate") {
		t.Errorf("the line does not open on navigate: %s", line)
	}
	if !strings.HasSuffix(line, "q Quit") {
		t.Errorf("the line does not end on quit, so quit is not the first entry a narrow terminal cuts: %s", line)
	}
}

// The keycap styling and the entry gap are what the journeys parse the footer by, so a renderer that drops either breaks the harness silently rather than visibly.
func TestOptionsEntriesAreSeparatedByTheThreeSpaceGap(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	line := renderedOptions(gui, "ec2")
	if strings.Contains(line, ",") {
		t.Errorf("the options line still uses comma separators: %s", line)
	}
	if got, want := strings.Count(line, optionsSeparator), len(gui.dashboardOptions("ec2"))-1; got != want {
		t.Errorf("the ec2 options line has %d three-space gaps, want one between each of its %d entries: %s", got, want+1, line)
	}
}

// The filter label and the filter BINDING answer to the same condition, and every shipped list is filterable, so this is the only way to drive the other side of it: without this the footer could advertise a key that does nothing on a future unfilterable panel.
func TestAnUnfilterablePanelDoesNotAdvertiseFilter(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	if line := renderedOptions(gui, "s3"); !strings.Contains(line, "Filter") {
		t.Fatalf("the s3 list is filterable but its footer does not say so, so this test cannot see the guard: %s", line)
	}

	gui.Panels.S3.DisableFilter = true

	if line := renderedOptions(gui, "s3"); strings.Contains(line, "Filter") {
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

	line := renderedOptions(gui, "ec2")
	if !strings.Contains(line, "Y Copy ARN") {
		t.Errorf("the footer ignores the rebound copy key: %s", line)
	}
	if !strings.Contains(line, "f Filter") {
		t.Errorf("the footer ignores the rebound filter key: %s", line)
	}
	if strings.Contains(line, "y Copy ARN") || strings.Contains(line, "/ Filter") {
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
		{view: gui.Views.EC2, want: "c Connect"},
		{view: gui.Views.Secrets, want: "v Reveal"},
		{view: gui.Views.ECS, want: "e Exec"},
	} {
		run(t, g, func() error { return gui.switchFocus(tc.view) })
		run(t, g, gui.renderPanelOptions)

		if line := waitForView(t, g, gui.Views.Options, tc.want); !strings.Contains(line, tc.want) {
			t.Errorf("with %s focused the footer reads %q, want it to offer %q", tc.view.Name(), line, tc.want)
		}
	}
}

// main is shared between the dashboard's detail pane and the chat's conversation, so the one view name has two footers.
// Advertising copy, tabs, select or actions in the chat names keys that do nothing there, which is worse than the generic line this replaced.
func TestTheMainFooterChangesOnTheChatScreen(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	if line := renderedOptions(gui, "main"); !strings.Contains(line, "y Copy ARN") {
		t.Fatalf("the dashboard's main line does not offer copy, so this test cannot see the difference: %s", line)
	}

	gui.setQScreenActive(true)
	t.Cleanup(func() { gui.setQScreenActive(false) })

	line := renderedOptions(gui, "main")
	if !strings.Contains(line, "Esc Dashboard") || !strings.Contains(line, "Tab Next pane") {
		t.Errorf("the chat's main line does not say how to get out or move on: %s", line)
	}
	for _, absent := range []string{"y Copy ARN", "[ ] Tabs", "Enter Select", "a Actions"} {
		if strings.Contains(line, absent) {
			t.Errorf("the chat's main line offers %q, which does nothing there: %s", absent, line)
		}
	}
}

// The footer shares one terminal row with the app status and the version, and gocui cuts what does not fit rather than wrapping.
// This is a budget, not an exact-width assertion: it fails when a future entry pushes the longest line past what a normal terminal shows, which is the point at which items start disappearing silently.
func TestNoDashboardOptionsLineOutgrowsTheFooter(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	const budget = 100

	for _, name := range append(sidePanelViewNames(gui.allSidePanels()), "main") {
		line := renderedOptions(gui, name)
		if width := runewidth.StringWidth(line); width > budget {
			t.Errorf("the %s options line is %d cells, over the %d-cell budget: %s", name, width, budget, line)
		}
	}
}
