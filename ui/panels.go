package ui

import "github.com/noelruault/lazyaws/ui/panels"

func (gui *Gui) intoInterface() panels.IGui {
	return gui
}

// swapPanelItems queues a reloaded list onto the UI loop and rerenders it there.
// The loop reads the items and the selection while it renders, so the swap cannot happen on the goroutine that fetched them; only the fetch belongs off the loop.
func swapPanelItems[T comparable](gui *Gui, panel *panels.SideListPanel[T], items []T, key func(T) string) {
	gui.Update(func() error {
		panel.SetItemsKeepSelection(items, key)

		return panel.RerenderList()
	})
}
