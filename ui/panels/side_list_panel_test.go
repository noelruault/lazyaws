package panels

import (
	"context"
	"strings"
	"testing"

	"github.com/fatih/color"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/ui/tasks"
)

// stubGui supplies only what FilterAndSort reads; the rest of IGui is never reached from these tests.
type stubGui struct {
	filter  string
	ignores []string
}

func (g *stubGui) FilterString(*gocui.View) string { return g.filter }
func (g *stubGui) IgnoreStrings() []string         { return g.ignores }

func (g *stubGui) HandleClick(*gocui.View, int, *int, func() error) error { return nil }
func (g *stubGui) NewSimpleRenderStringTask(func() string) tasks.TaskFunc { return nil }
func (g *stubGui) FocusY(int, int, *gocui.View)                           {}
func (g *stubGui) ShouldRefresh(string) bool                              { return true }
func (g *stubGui) GetMainView() *gocui.View                               { return nil }
func (g *stubGui) IsCurrentView(*gocui.View) bool                         { return false }
func (g *stubGui) Update(func() error)                                    {}
func (g *stubGui) QueueTask(func(context.Context)) error                  { return nil }

// An item matches the filter or an ignore rule if ANY of its rendered cells does, so a match on the second column must count exactly as much as one on the first.
func TestFilterAndSortMatchesAcrossEveryCell(t *testing.T) {
	items := []string{"bastion", "web-server-1", "worker", "db-primary"}

	for _, tt := range []struct {
		name    string
		filter  string
		ignores []string
		want    []string
	}{
		{"no filter keeps everything", "", nil, items},
		{"filter matches the first cell", "work", nil, []string{"worker"}},
		{"filter matches the trailing cell only", "tagged-db-primary", nil, []string{"db-primary"}},
		{"filter matching nothing empties the list", "zzz", nil, nil},
		{"ignore drops a matching item", "", []string{"bastion"}, []string{"web-server-1", "worker", "db-primary"}},
		{"ignore matches the trailing cell too", "", []string{"tagged-worker"}, []string{"bastion", "web-server-1", "db-primary"}},
		{"any of several ignores is enough", "", []string{"nope", "worker"}, []string{"bastion", "web-server-1", "db-primary"}},
		{"ignore wins over a filter that would keep the item", "worker", []string{"worker"}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			panel := &SideListPanel[string]{
				ListPanel:     ListPanel[string]{List: NewFilteredList[string]()},
				Gui:           &stubGui{filter: tt.filter, ignores: tt.ignores},
				GetTableCells: func(item string) []string { return []string{item, "tagged-" + item} },
			}
			panel.List.SetItems(items)

			panel.FilterAndSort()

			got := panel.List.GetItems()
			if len(got) != len(tt.want) {
				t.Fatalf("items = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("item %d = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// The panel's own Filter runs before ignore and filter strings, and rejecting there is final.
func TestFilterAndSortHonoursThePanelFilter(t *testing.T) {
	panel := &SideListPanel[string]{
		ListPanel:     ListPanel[string]{List: NewFilteredList[string]()},
		Gui:           &stubGui{},
		GetTableCells: func(item string) []string { return []string{item} },
		Filter:        func(item string) bool { return item != "hidden" },
	}
	panel.List.SetItems([]string{"shown", "hidden", "also-shown"})

	panel.FilterAndSort()

	got := panel.List.GetItems()
	if len(got) != 2 || got[0] != "shown" || got[1] != "also-shown" {
		t.Errorf("items = %v, want [shown also-shown]", got)
	}
}

func TestGetMainTabTitles(t *testing.T) {
	state := &ContextState[string]{
		GetMainTabs: func() []MainTab[string] {
			return []MainTab[string]{
				{Key: "logs", Title: "Logs"},
				{Key: "config", Title: "Config"},
			}
		},
	}

	got := state.GetMainTabTitles()
	if len(got) != 2 || got[0] != "Logs" || got[1] != "Config" {
		t.Errorf("GetMainTabTitles() = %v, want [Logs Config]", got)
	}

	state.GetMainTabs = func() []MainTab[string] { return nil }
	if got := state.GetMainTabTitles(); len(got) != 0 {
		t.Errorf("GetMainTabTitles() with no tabs = %v, want empty", got)
	}
}

// forceColor makes the styling observable: a test binary is not a tty, so fatih/color otherwise strips every escape and any assertion about muting passes vacuously.
func forceColor(t *testing.T) {
	t.Helper()

	previous := color.NoColor
	color.NoColor = false
	t.Cleanup(func() { color.NoColor = previous })
}

// gocui turns escapes into cell attributes, so the muting cannot be read back off the view: assert it on the string the panel writes.
func TestEmptyMessageIsMuted(t *testing.T) {
	forceColor(t)

	panel := &SideListPanel[string]{NoItemsMessage: "no EKS clusters"}

	got := panel.emptyMessage()
	want := color.New(color.Faint).Sprint("no EKS clusters")
	if got != want {
		t.Errorf("emptyMessage() = %q, want %q", got, want)
	}
	if !strings.Contains(got, "no EKS clusters") {
		t.Errorf("emptyMessage() = %q, want it to still carry the message text", got)
	}
}

// The menu panel deliberately has no message, and an empty view is better than a stray escape pair.
func TestEmptyMessageIsBlankWhenThePanelHasNoMessage(t *testing.T) {
	forceColor(t)

	panel := &SideListPanel[string]{}

	if got := panel.emptyMessage(); got != "" {
		t.Errorf("emptyMessage() = %q, want the empty string", got)
	}
}
