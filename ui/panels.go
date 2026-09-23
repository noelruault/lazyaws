package ui

import "github.com/noelruault/lazyaws/ui/panels"

func (gui *Gui) intoInterface() panels.IGui {
	return gui
}

// swapPanelItems queues the swap onto the UI loop, which reads the items and the selection while it renders; only the fetch belongs off the loop.
// gen is rechecked here rather than by the caller because the profile can be switched between the fetch returning and this closure running, which would put one account's rows on another account's panel.
func swapPanelItems[T comparable](gui *Gui, gen int64, panel *panels.SideListPanel[T], items []T, key func(T) string) {
	gui.queueUpdate(func() error {
		if gen != gui.Generation() {
			return nil
		}

		panel.SetItemsKeepSelection(items, key)

		return panel.RerenderList()
	})
}
