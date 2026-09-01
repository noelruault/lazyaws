package ui

import (
	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/config"
	"github.com/noelruault/lazyaws/ui/types"
)

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

	bindingsPanel = append(bindingsPanel, &Binding{})
	return append(bindingsPanel, bindingsGlobal...)
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

	menuItems := gui.mergeByDescription(gui.getBindings(v), g, v)

	// Every row above is rebindable, and nothing else on screen says so; the file is the same one the Settings screen writes, and `o` then `e` opens it.
	menuItems = append(menuItems,
		&types.MenuItem{LabelColumns: []string{"", "rebind any of these under keybindings: in " + config.ConfigFilename() + ", or press o then e to open it"}},
	)

	return gui.Menu(CreateMenuOptions{
		Title:      "Menu",
		Items:      menuItems,
		HideCancel: true,
	})
}
