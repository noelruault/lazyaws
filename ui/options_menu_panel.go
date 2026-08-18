// Adapted from lazydocker's options menu (MIT, © 2018 Jesse Duffield).
package ui

import (
	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/ui/types"
)

func (gui *Gui) getBindings(v *gocui.View) []*Binding {
	var bindingsGlobal, bindingsPanel []*Binding

	bindings := gui.GetInitialKeybindings()

	for _, binding := range bindings {
		if binding.GetKey() != "" && binding.Description != "" {
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

func (gui *Gui) handleCreateOptionsMenu(g *gocui.Gui, v *gocui.View) error {
	if gui.isPopupPanel(v.Name()) {
		return nil
	}

	bindings := gui.getBindings(v)
	menuItems := make([]*types.MenuItem, len(bindings))
	for i, binding := range bindings {
		menuItems[i] = &types.MenuItem{
			LabelColumns: []string{binding.GetKey(), binding.Description},
			OnPress: func() error {
				if binding.Key == nil {
					return nil
				}
				return binding.Handler(g, v)
			},
		}
	}

	return gui.Menu(CreateMenuOptions{
		Title:      "Menu",
		Items:      menuItems,
		HideCancel: true,
	})
}
