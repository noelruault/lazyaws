package ui

import (
	"strings"
	"testing"
)

// Two swaps enqueued back to back have to land in that order, or the reload that answered second would leave the panel holding the first one's rows.
// gocui's own Update cannot promise that, which is why the swap goes through queueUpdate.
func TestSwapPanelItemsLandsInTheOrderItWasEnqueued(t *testing.T) {
	gui, g := newHeadlessGui(t)
	gen := gui.Generation()

	swapPanelItems(gui, gen, gui.Panels.Profile, []string{"first-only"}, profileSelectionKey)
	swapPanelItems(gui, gen, gui.Panels.Profile, []string{"second-a", "second-b"}, profileSelectionKey)

	got := ask(g, func() []string { return gui.Panels.Profile.List.GetItems() })
	if want := "second-a second-b"; strings.Join(got, " ") != want {
		t.Errorf("the panel holds %q, want %q: the swaps applied out of order", strings.Join(got, " "), want)
	}
}

// A result fetched under one profile must not land on the panel of another, and the switch can happen after the fetch returns, so the generation is what the swap checks on the loop.
func TestSwapPanelItemsDropsASupersededResult(t *testing.T) {
	gui, g := newHeadlessGui(t)
	gen := gui.Generation()

	swapPanelItems(gui, gen, gui.Panels.Profile, []string{"live"}, profileSelectionKey)

	// Read on the loop, which is what puts the first swap behind us before the generation moves.
	if rows := ask(g, func() int { return gui.Panels.Profile.List.Len() }); rows != 1 {
		t.Fatalf("the live swap left %d rows, want 1", rows)
	}

	gui.BumpGeneration()
	swapPanelItems(gui, gen, gui.Panels.Profile, []string{"stale"}, profileSelectionKey)

	got := ask(g, func() []string { return gui.Panels.Profile.List.GetItems() })
	if len(got) != 1 || got[0] != "live" {
		t.Errorf("the panel holds %q, want the rows from the live generation", got)
	}
}
