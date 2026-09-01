package ui

import (
	"strings"
	"testing"

	"github.com/noelruault/lazyaws/config"
)

// The footer is one hint now, so the menu is the whole map of the keyboard: a key that works in a view and is missing here is a key nothing on screen mentions.
func TestTheMenuListsEveryKeyTheFocusedViewAnswersTo(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, func() error { return gui.switchFocus(gui.Views.EC2) })
	run(t, g, func() error { return gui.handleCreateOptionsMenu(g, gui.Views.EC2) })

	menu := readView(g, gui.Views.Menu)

	// Keycaps as GetKey renders them: the arrows are the triangles gocui draws, and Home, End and shift+tab had no label at all until they were named, which kept them out of this list entirely.
	for _, want := range []string{
		"▲", "▼",
		"►",         // leaves the list for the main panel
		"tab",       // walks the panel column
		"shift+tab", //
		"enter",     // looks into the selection
		"esc",       // steps back
		"c",         // an EC2 key, which is what makes this the EC2 menu
		"y", "a", "/", "r", "q",
		"PgUp", "PgDn", "Home", "End", "ctrl+c",
	} {
		if !strings.Contains(menu, want) {
			t.Errorf("the EC2 menu does not list %q:\n%s", want, menu)
		}
	}
}

// Every key here is rebindable and the menu is where a user goes looking, so it has to say so and name the file.
func TestTheMenuSaysWhereTheKeysAreRebound(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, func() error { return gui.switchFocus(gui.Views.EC2) })
	run(t, g, func() error { return gui.handleCreateOptionsMenu(g, gui.Views.EC2) })

	menu := readView(g, gui.Views.Menu)
	if !strings.Contains(menu, "keybindings:") {
		t.Errorf("the menu never mentions the config key that rebinds these:\n%s", menu)
	}
	if !strings.Contains(menu, config.ConfigFilename()) {
		t.Errorf("the menu does not name the file to rebind them in (%s):\n%s", config.ConfigFilename(), menu)
	}
}

// An arrow and its vim key run the same handler, so listing them apart says the same sentence twice and makes the menu look like it holds two different keys.
func TestTheMenuPutsAKeyAndItsAliasOnOneRow(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, func() error { return gui.switchFocus(gui.Views.EC2) })
	items := ask(g, func() []string {
		rows := []string{}
		for _, item := range gui.mergeByDescription(gui.getBindings(gui.Views.EC2), g, gui.Views.EC2) {
			rows = append(rows, strings.Join(item.LabelColumns, "\x00"))
		}
		return rows
	})

	var up, help int
	for _, row := range items {
		key, description, _ := strings.Cut(row, "\x00")
		if description == gui.Keys.Get(KeyNavUp).Description {
			up++
			if !strings.Contains(key, "k") || !strings.Contains(key, "▲") {
				t.Errorf("the move up row is %q, want both k and the arrow on it", key)
			}
		}
		if description == gui.Keys.Get(KeyHelp).Description {
			help++
			if !strings.Contains(key, "x") || !strings.Contains(key, "?") {
				t.Errorf("the menu row is %q, want both x and ? on it", key)
			}
		}
	}

	if up != 1 {
		t.Errorf("move up appears on %d rows, want 1", up)
	}
	if help != 1 {
		t.Errorf("the keybindings menu appears on %d rows, want 1", help)
	}
}
