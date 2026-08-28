package presentation

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/ui/utils"
)

// The gallery is the design-decision surface, so what it must never do is lie by omission: every shared component appears, both tag styles appear, and no line outgrows the width it was asked for.
func TestGalleryShowsEveryComponentInsideItsWidth(t *testing.T) {
	forceColor(t)

	const width = 110
	got := Gallery(width)
	plain := utils.Decolorise(got)

	for _, want := range []string{
		"ResourceHeader", "StatBoxes(compact)", "StatBoxes(filled)",
		"statCardsOn", "BoxedTable", "boxedTablesOn", "frameless", "Badge", "Gauge", "kvBlock", "SectionTitle",
		"tagChips", "plain lines",
		// One recognisable rendering per component, so a caption without its block cannot pass.
		"app-cluster", "Services: 1 / 1", "● ACTIVE", "Cluster:     ● ACTIVE", "▦ Service Summary", "app:42",
		"● some-new-state", "▕███", "Container Insights:", "⌘ Console",
		"│ Environment: staging │", "Environment: staging\nTeam: security",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("the gallery does not show %q\n%s", want, plain)
		}
	}

	for i, line := range strings.Split(plain, "\n") {
		if w := runewidth.StringWidth(line); w > width {
			t.Errorf("gallery line %d is %d cells, over the %d-cell width: %q", i, w, width, line)
		}
	}
}

// The gallery forces every style for its samples and must put the switches back, or looking at the gallery would change how the app renders.
func TestGalleryLeavesStyleSwitchesAlone(t *testing.T) {
	previousStats := statCardsOn
	previousTables := boxedTablesOn
	previousTags := tagStyleChips
	t.Cleanup(func() {
		statCardsOn = previousStats
		boxedTablesOn = previousTables
		tagStyleChips = previousTags
	})

	statCardsOn = false
	boxedTablesOn = false
	tagStyleChips = false
	Gallery(80)
	if statCardsOn != false {
		t.Error("rendering the gallery flipped statCardsOn")
	}
	if boxedTablesOn != false {
		t.Error("rendering the gallery flipped boxedTablesOn")
	}
	if tagStyleChips != false {
		t.Error("rendering the gallery flipped tagStyleChips")
	}
}

// tagsBody is the one routing point for every pane's tag section, so the style switch has to actually switch it.
func TestTagsBodyFollowsTheStyleSwitch(t *testing.T) {
	previous := tagStyleChips
	t.Cleanup(func() { tagStyleChips = previous })

	tags := []kv{{"Environment", "staging"}}

	tagStyleChips = true
	if got := utils.Decolorise(tagsBody(80, tags)); !strings.Contains(got, "┌") {
		t.Errorf("chip style renders no border: %q", got)
	}

	tagStyleChips = false
	got := utils.Decolorise(tagsBody(80, tags))
	if strings.Contains(got, "┌") {
		t.Errorf("line style still renders a border: %q", got)
	}
	if got != "Environment: staging" {
		t.Errorf("line style = %q, want %q", got, "Environment: staging")
	}
}
