package ui

import (
	"strings"
	"testing"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/config"
	"github.com/noelruault/lazyaws/ui/types"
	"github.com/noelruault/lazyaws/ui/utils"
)

// menuGroupTitle names the group a heading row opens, and returns "" for every other row.
// A heading is recognised by its second column being the rule, because the first column carries a colour and cannot be compared against a plain title.
func menuGroupTitle(item *types.MenuItem) string {
	if len(item.LabelColumns) != 2 {
		return ""
	}
	if utils.Decolorise(item.LabelColumns[1]) != strings.Repeat("─", menuRuleWidth) {
		return ""
	}

	return utils.Decolorise(item.LabelColumns[0])
}

// The footer is one hint now, so the menu is the whole map of the keyboard: a key that works in a view and is missing here is a key nothing on screen mentions.
func TestTheMenuListsEveryKeyTheFocusedViewAnswersTo(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, func() error { return gui.switchFocus(gui.Views.EC2) })
	run(t, g, func() error { return gui.handleCreateOptionsMenu(g, gui.Views.EC2) })

	// WAITED rather than read once: the menu paints through gocui's Update queue, and the headless screen is shared with every other test in this package, so a single read can land before the rows arrive.
	menu := waitForView(t, g, gui.Views.Menu, "ctrl+c")

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

// Every key here is rebindable and the menu is where a user goes looking, so it has to say so and point at both routes.
// The path itself is deliberately absent: it is long enough to set the popup's width on its own, and `lazyaws --keymap` prints it on demand.
func TestTheMenuSaysWhereTheKeysAreRebound(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, func() error { return gui.switchFocus(gui.Views.EC2) })
	run(t, g, func() error { return gui.handleCreateOptionsMenu(g, gui.Views.EC2) })

	menu := utils.Decolorise(waitForView(t, g, gui.Views.Menu, "REBIND"))
	for _, want := range []string{"o then e", "--keymap"} {
		if !strings.Contains(menu, want) {
			t.Errorf("the menu does not offer %q as a way to rebind these keys:\n%s", want, menu)
		}
	}
	if strings.Contains(menu, config.ConfigFilename()) {
		t.Errorf("the menu spells out the config path, which the flag reports instead:\n%s", menu)
	}
}

// The menu's order is a table, menuGroups, and this is what keeps that table honest: every key the view answers to lands in a titled group, the groups appear in the declared order, and nothing falls through to the catch-all.
func TestTheMenuGroupsKeysByWhatTheyDo(t *testing.T) {
	gui, g := newHeadlessGui(t)

	for _, view := range []*gocui.View{gui.Views.ECS, gui.Views.Main, gui.Views.Secrets} {
		t.Run(view.Name(), func(t *testing.T) {
			run(t, g, func() error { return gui.switchFocus(view) })
			items := ask(g, func() []*types.MenuItem {
				return gui.groupForMenu(gui.getBindings(view), g, view)
			})

			var titles []string
			for _, item := range items {
				if title := menuGroupTitle(item); title != "" {
					titles = append(titles, title)
				}
			}

			// A group with no rows is skipped, so the titles present must be a subsequence of the declared order rather than all of it.
			declared := []string{}
			for _, group := range menuGroups {
				declared = append(declared, group.title)
			}
			at := 0
			for _, title := range titles {
				if title == "everything else" {
					t.Errorf("%s: some keys reached the catch-all group, so menuGroups is missing them: %v", view.Name(), titles)
					continue
				}
				found := false
				for ; at < len(declared); at++ {
					if declared[at] == title {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s: group %q is out of order or unknown; want a subsequence of %v, got %v", view.Name(), title, declared, titles)
				}
			}

			if len(titles) == 0 || titles[0] != menuGroups[0].title {
				t.Errorf("%s: the menu does not open on %q: %v", view.Name(), menuGroups[0].title, titles)
			}

			// Scattering movement is what this ordering exists to prevent, so the cursor and paging keys have to sit inside the first group rather than anywhere in the list.
			inMoving := false
			moving := map[string]bool{}
			for _, item := range items {
				if title := menuGroupTitle(item); title != "" {
					inMoving = title == menuGroups[0].title
					continue
				}
				keys := utils.Decolorise(item.LabelColumns[0])
				if keys == "" {
					continue // the blank row between groups
				}
				if inMoving {
					for _, key := range strings.Split(keys, " / ") {
						moving[key] = true
					}
				}
			}

			for _, key := range []string{"▲", "▼", "PgUp", "PgDn", "Home", "End"} {
				if !moving[key] {
					t.Errorf("%s: %q is not in the %q group: %v", view.Name(), key, menuGroups[0].title, moving)
				}
			}
		})
	}
}

// The menu is the only place the keys are written down, so it has to describe the layout in force rather than the one that shipped.
// Read from KeyPresets, so a new layout is covered here the moment it exists.
func TestTheMenuShowsTheKeysOfTheActivePreset(t *testing.T) {
	for _, preset := range PresetNames() {
		t.Run(preset, func(t *testing.T) {
			user := config.DefaultUserConfig()
			user.KeybindingPreset = preset
			gui, g := newHeadlessGuiWithConfig(t, user)

			view := gui.Views.ECS
			run(t, g, func() error { return gui.switchFocus(view) })
			rows := ask(g, func() map[string]string {
				keys := map[string]string{}
				for _, item := range gui.mergeByDescription(gui.getBindings(view), g, view) {
					keys[item.LabelColumns[1]] = item.LabelColumns[0]
				}
				return keys
			})

			// Moving up the list and changing detail tab are what a preset is mostly about, and both are bound on a resource panel, so they are what this asserts on the rendered rows.
			// Only these two, because a preset's other moves land on views this menu is not showing: the chat's keys belong to the chat panes.
			for _, name := range []KeyName{KeyNavUp, KeyNextTab} {
				chord := describeKey(gui.Keys.Get(name).Key)
				found := false
				for _, shown := range rows {
					for _, part := range strings.Split(shown, " / ") {
						if part == chord {
							found = true
						}
					}
				}
				if !found {
					t.Errorf("preset %q binds %s to %q, which the menu does not list: %v", preset, name, chord, rows)
				}
			}

			// Nothing the menu prints may be invented locally: every named binding in this view has to carry the chord the active keymap resolved.
			for _, binding := range gui.GetInitialKeybindings() {
				if binding.Name == "" || (binding.ViewName != "" && binding.ViewName != view.Name()) {
					continue
				}
				if want := gui.Keys.Get(binding.Name); binding.Key != want.Key || binding.Modifier != want.Modifier {
					t.Errorf("preset %q: %s is bound to %v in the menu but the keymap says %v", preset, binding.Name, binding.Key, want.Key)
				}
			}
		})
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
