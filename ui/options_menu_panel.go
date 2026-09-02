package ui

import (
	"strings"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/ui/types"
	"github.com/noelruault/lazyaws/ui/utils"
)

// menuRuleWidth is the heading rule's length, wide enough to read as a divider and narrow enough not to set the popup's width.
const menuRuleWidth = 28

func (gui *Gui) getBindings(v *gocui.View) []*Binding {
	var bindingsGlobal, bindingsPanel []*Binding

	bindings := gui.GetInitialKeybindings()

	// gocui dispatch stops at the first binding registered for a view and a key, so the menu lists that one and drops what it shadows: ECS registers Enter as "drill down" ahead of the generic "go to the main panel", and listing both would offer a row Enter can never reach.
	listed := map[string]bool{}
	claim := func(binding *Binding) bool {
		seen := binding.ViewName + "\x00" + binding.GetKey()
		if listed[seen] {
			return false
		}
		listed[seen] = true

		return true
	}

	for _, binding := range bindings {
		if binding.GetKey() != "" && binding.Description != "" && claim(binding) {
			switch binding.ViewName {
			case "":
				bindingsGlobal = append(bindingsGlobal, binding)
			case v.Name():
				bindingsPanel = append(bindingsPanel, binding)
			}
		}
	}

	if v.ParentView != nil {
	L:
		for _, binding := range bindings {
			if binding.GetKey() != "" && binding.Description != "" {
				if binding.ViewName == v.ParentView.Name() {
					// View-local bindings must win conflicts with their parent.
					for _, ownBinding := range bindingsPanel {
						if ownBinding.GetKey() == binding.GetKey() {
							continue L
						}
					}
					bindingsPanel = append(bindingsPanel, binding)
				}
			}
		}
	}

	// A global binding whose key this view already claims is dropped: gocui matches the view first, so ECS listing both "esc drill up" and the global "esc close what is open" would advertise one key twice, and the second of them never runs here.
	claimedHere := map[string]bool{}
	for _, binding := range bindingsPanel {
		claimedHere[binding.GetKey()] = true
	}

	// The two scopes are concatenated rather than kept apart, because the menu is grouped by what a key DOES: this menu only ever lists keys that work in the focused view, so "here" against "everywhere" was a distinction without a use, and it split movement across two places.
	for _, binding := range bindingsGlobal {
		if !claimedHere[binding.GetKey()] {
			bindingsPanel = append(bindingsPanel, binding)
		}
	}

	return bindingsPanel
}

// menuGroup is a run of rows the menu keeps together, in the order menuGroups lists them.
// Membership is by KeyName because a name is stable while its chord is not, and by keycap for the literals config cannot move.
// Keep a title under about twelve columns: it renders in the keycap column, which is padded to its widest cell, so a longer one pushes every description right.
type menuGroup struct {
	title string
	names []KeyName
	keys  []string
}

// menuGroups is where the menu's ordering lives. Reordering this reorders the menu; nothing else does.
// It is deliberately separate from the registration order in GetInitialKeybindings, because that order is also gocui's dispatch order: moving an entry there changes which handler wins a shared key, and grouping the help text should never do that.
var menuGroups = []menuGroup{
	{
		title: "MOVING",
		names: []KeyName{
			KeyNavUp, KeyNavDown, KeyNavLeft, KeyNavRight,
			KeyPrevTab, KeyNextTab,
			KeyScrollMainUp, KeyScrollMainDown, KeyScrollMainPageUp, KeyScrollMainPageDown,
		},
		keys: []string{"◄", "►", "▲", "▼", "tab", "shift+tab", "Home", "End"},
	},
	{
		title: "INSPECTING",
		names: []KeyName{KeyCopyID, KeyFilter, KeySecretsReveal, KeySecretsToggleDeleted},
		keys:  []string{"enter", "esc"},
	},
	{
		title: "ACTIONS",
		names: []KeyName{KeyActions, KeyCommandBar, KeyECSExec, KeyEC2Connect, KeyRefreshPanel, KeyRefreshAll},
	},
	{
		title: "THE APP",
		names: []KeyName{
			KeyOptionsMenu, KeyHelp, KeySettings, KeySettingsEditFile, KeyAmazonQ,
			KeyScreenModeNext, KeyScreenModePrev, KeyRedraw, KeyQuit,
			KeyChatPickModel, KeyChatNewConversation, KeyChatToggleFolds,
		},
		keys: []string{"1", "2", "3", "4", "5", "6", "7", "8", "ctrl+c"},
	},
}

// holds reports whether a binding belongs to this group, by its configurable name first and its printed keycap second.
func (group menuGroup) holds(binding *Binding) bool {
	for _, name := range group.names {
		if binding.Name == name {
			return true
		}
	}

	for _, key := range group.keys {
		if binding.GetKey() == key {
			return true
		}
	}

	return false
}

// groupForMenu lays the bindings out in menuGroups order, each run behind its title, and keeps anything no group claims: a key the menu drops is a key nothing on screen mentions.
func (gui *Gui) groupForMenu(bindings []*Binding, g *gocui.Gui, v *gocui.View) []*types.MenuItem {
	remaining := bindings
	items := []*types.MenuItem{}

	appendGroup := func(title string, picked []*Binding) {
		if len(picked) == 0 {
			return
		}
		if len(items) > 0 {
			items = append(items, &types.MenuItem{LabelColumns: []string{"", ""}})
		}
		items = append(items, &types.MenuItem{LabelColumns: []string{
			utils.ColoredString(title, color.FgCyan),
			// A rule rather than nothing, so the row reads as a heading and not as a key whose description went missing. Shorter than the longest description on purpose: the popup sizes itself to its widest row, and a rule has no business deciding how wide the menu is.
			utils.ColoredString(strings.Repeat("─", menuRuleWidth), color.Faint),
		}})
		items = append(items, gui.mergeByDescription(picked, g, v)...)
	}

	for _, group := range menuGroups {
		var picked, rest []*Binding
		for _, binding := range remaining {
			if group.holds(binding) {
				picked = append(picked, binding)
				continue
			}
			rest = append(rest, binding)
		}
		remaining = rest

		appendGroup(group.title, picked)
	}

	appendGroup("everything else", remaining)

	return items
}

// mergeByDescription puts every key that does the same thing on one row, so the arrows sit beside their vim keys and the menu says each action once.
// Only neighbours are merged: the list arrives grouped by view, and a description repeated in two different groups is two different actions ("next pane" in the chat is not "next pane" on the dashboard).
func (gui *Gui) mergeByDescription(bindings []*Binding, g *gocui.Gui, v *gocui.View) []*types.MenuItem {
	items := make([]*types.MenuItem, 0, len(bindings))
	previous := map[string]*types.MenuItem{}

	for _, binding := range bindings {
		key := binding.GetKey()
		if key == "" {
			// The blank row getBindings inserts divides this view's own keys from the ones bound everywhere, and a bare gap reads as a rendering fault, so it says which it is.
			// It also ends the run: a description either side of it belongs to a different group.
			items = append(items, &types.MenuItem{LabelColumns: []string{"", "the keys below work in every view"}})
			previous = map[string]*types.MenuItem{}
			continue
		}

		if item, ok := previous[binding.Description]; ok {
			item.LabelColumns[0] += " / " + key
			continue
		}

		item := &types.MenuItem{
			LabelColumns: []string{key, binding.Description},
			OnPress: func() error {
				if binding.Key == nil {
					return nil
				}
				return binding.Handler(g, v)
			},
		}
		items = append(items, item)
		previous[binding.Description] = item
	}

	return items
}

func (gui *Gui) handleCreateOptionsMenu(g *gocui.Gui, v *gocui.View) error {
	if gui.isPopupPanel(v.Name()) {
		return nil
	}

	menuItems := gui.groupForMenu(gui.getBindings(v), g, v)

	// Every row above is rebindable and nothing else on screen says so. The path is deliberately absent: it differs per platform, it is long enough to have been truncated here, and `lazyaws --keymap` prints it.
	menuItems = append(menuItems,
		&types.MenuItem{LabelColumns: []string{"", ""}},
		&types.MenuItem{LabelColumns: []string{
			utils.ColoredString("REBIND", color.FgCyan),
			utils.ColoredString("o then e edits these keys, lazyaws --keymap says where they live", color.Faint),
		}},
	)

	return gui.Menu(CreateMenuOptions{
		Title:      "Menu",
		Items:      menuItems,
		HideCancel: true,
	})
}
