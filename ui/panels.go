package ui

import "github.com/noelruault/lazyaws/ui/panels"

func (gui *Gui) intoInterface() panels.IGui {
	return gui
}

// swapPanelItems queues the swap onto the UI loop, which reads the items and the selection while it renders; only the fetch belongs off the loop.
func swapPanelItems[T comparable](gui *Gui, panel *panels.SideListPanel[T], items []T, key func(T) string) {
	gui.Update(func() error {
		panel.SetItemsKeepSelection(items, key)

		return panel.RerenderList()
	})
}
