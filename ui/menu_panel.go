// menu_panel.go — the lazyaws port of lazydocker's pkg/gui/menu_panel.go (MIT, © 2018 Jesse Duffield).
package ui

import (
	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/presentation"
	"github.com/noelruault/lazyaws/ui/types"
	"github.com/noelruault/lazyaws/ui/utils"
)

type CreateMenuOptions struct {
	Title      string
	Items      []*types.MenuItem
	HideCancel bool
}

// dropMutatingItems is the shared menu boundary for read-only visibility.
func (gui *Gui) dropMutatingItems(items []*types.MenuItem) []*types.MenuItem {
	if !gui.readOnly() {
		return items
	}

	kept := make([]*types.MenuItem, 0, len(items))
	for _, item := range items {
		if !item.Mutates {
			kept = append(kept, item)
		}
	}

	return kept
}

func (gui *Gui) getMenuPanel() *panels.SideListPanel[*types.MenuItem] {
	return &panels.SideListPanel[*types.MenuItem]{
		ListPanel: panels.ListPanel[*types.MenuItem]{
			List: panels.NewFilteredList[*types.MenuItem](),
			View: gui.Views.Menu,
		},
		NoItemsMessage: "",
		Gui:            gui.intoInterface(),
		OnClick:        gui.onMenuPress,
		Sort:           nil,
		GetTableCells:  presentation.GetMenuItemDisplayStrings,
		OnRerender: func() error {
			return gui.resizePopupPanel(gui.Views.Menu)
		},
		DisableFilter: true,
	}
}

func (gui *Gui) onMenuPress(menuItem *types.MenuItem) error {
	if err := gui.handleMenuClose(); err != nil {
		return err
	}

	if menuItem.OnPress != nil {
		return menuItem.OnPress()
	}

	return nil
}

func (gui *Gui) handleMenuPress() error {
	selectedMenuItem, err := gui.Panels.Menu.GetSelectedItem()
	if err != nil {
		return nil
	}

	return gui.onMenuPress(selectedMenuItem)
}

func (gui *Gui) Menu(opts CreateMenuOptions) error {
	opts.Items = gui.dropMutatingItems(opts.Items)

	if !opts.HideCancel {
		opts.Items = append(opts.Items, &types.MenuItem{
			LabelColumns: []string{"cancel"},
			OnPress: func() error {
				return nil
			},
		})
	}

	maxColumnSize := 1

	for _, item := range opts.Items {
		if item.LabelColumns == nil {
			item.LabelColumns = []string{item.Label}
		}

		if item.OpensMenu {
			item.LabelColumns[0] = utils.OpensMenuStyle(item.LabelColumns[0])
		}

		maxColumnSize = max(maxColumnSize, len(item.LabelColumns))
	}

	for _, item := range opts.Items {
		if len(item.LabelColumns) < maxColumnSize {
			// RenderTable requires every row to have equal width.
			item.LabelColumns = append(item.LabelColumns, make([]string, maxColumnSize-len(item.LabelColumns))...)
		}
	}

	gui.Panels.Menu.SetItems(opts.Items)
	gui.Panels.Menu.SetSelectedLineIdx(0)

	if err := gui.Panels.Menu.RerenderList(); err != nil {
		return err
	}

	gui.Views.Menu.Title = opts.Title
	gui.Views.Menu.Visible = true

	return gui.switchFocus(gui.Views.Menu)
}

func (gui *Gui) renderMenuOptions() error {
	optionsMap := map[string]string{
		"esc":   "close",
		"↑ ↓":   "navigate",
		"enter": "execute",
	}
	return gui.renderOptionsMap(optionsMap)
}

func (gui *Gui) handleMenuClose() error {
	gui.Views.Menu.Visible = false

	if gui.State.Filter.panel == gui.Panels.Menu {
		if err := gui.clearFilter(); err != nil {
			return err
		}

		// Remove the filter before restoring focus so it cannot regain focus after deletion.
		gui.removeViewFromStack(gui.Views.Filter)
	}

	return gui.returnFocus()
}
