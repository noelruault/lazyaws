package ui

import (
	"github.com/jesseduffield/gocui"
)

// syncDim runs in layout and preserves foregrounds so queued popups and themes restore correctly.
func (gui *Gui) syncDim() {
	if gui.g == nil {
		return
	}

	// Turning the setting off mid-session has to give back anything already faded, so this is a restore rather than a return.
	wantDim := gui.Config.User.Gui.DimBehindPopups && gui.popupVisible()

	if gui.dimmed == nil {
		gui.dimmed = map[string]gocui.Attribute{}
	}

	for _, view := range gui.g.Views() {
		if gui.isPopupPanel(view.Name()) {
			continue
		}

		_, faded := gui.dimmed[view.Name()]

		switch {
		case wantDim && !faded:
			gui.dimmed[view.Name()] = view.FgColor
			view.FgColor |= gocui.AttrDim
		case !wantDim && faded:
			view.FgColor = gui.dimmed[view.Name()]
			delete(gui.dimmed, view.Name())
		}
	}
}

func (gui *Gui) popupVisible() bool {
	for _, name := range gui.popupViewNames() {
		view, err := gui.g.View(name)
		if err == nil && view.Visible {
			return true
		}
	}

	return false
}
