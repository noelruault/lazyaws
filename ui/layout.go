// layout.go — the lazyaws port of lazydocker's pkg/gui/layout.go (MIT, © 2018 Jesse Duffield).
package ui

import (
	"github.com/jesseduffield/gocui"
)

func (gui *Gui) layout(g *gocui.Gui) error {
	g.Highlight = true
	width, height := g.Size()

	appStatus := gui.statusManager.getStatusString()

	viewDimensions := gui.getWindowDimensions(gui.getInformationContent(), appStatus)

	// createAllViews runs before this manager, so every mapping must resolve to an existing view.
	setViewFromDimensions := func(viewName string) (*gocui.View, error) {
		dimensionsObj, ok := viewDimensions[viewName]

		view, err := g.View(viewName)
		if err != nil {
			return nil, err
		}

		if !ok {
			// Hidden views retain screen-sized dimensions so lazy content measures correctly before appearing.
			_, err := g.SetView(viewName, 0, 0, width, height, 0)
			view.Visible = false
			return view, err
		}

		frameOffset := 1
		if view.Frame {
			frameOffset = 0
		}
		_, err = g.SetView(
			viewName,
			dimensionsObj.X0-frameOffset,
			dimensionsObj.Y0-frameOffset,
			dimensionsObj.X1+frameOffset,
			dimensionsObj.Y1+frameOffset,
			0,
		)
		view.Visible = true

		return view, err
	}

	for _, viewName := range gui.autoPositionedViewNames() {
		_, err := setViewFromDimensions(viewName)
		if err != nil && !gocui.IsUnknownView(err) {
			return err
		}
	}

	// Chat rewraps here because layout owns race-free access to the new width.
	gui.syncQWidth()

	// Dimming belongs to frame state, so layout owns it instead of popup creation.
	gui.syncDim()

	return gui.resizeCurrentPopupPanel(g)
}

func (gui *Gui) focusPointInView(view *gocui.View) {
	if view == nil {
		return
	}

	for _, panel := range gui.allListPanels() {
		if panel.GetView() == view {
			panel.Refocus()
			return
		}
	}
}

func (gui *Gui) getFocusLayout() func(g *gocui.Gui) error {
	var previousView *gocui.View
	return func(g *gocui.Gui) error {
		newView := gui.g.CurrentView()
		if err := gui.onFocusChange(); err != nil {
			return err
		}
		// Popups temporarily overlay their parent and must not trigger its focus-loss lifecycle.
		if newView != previousView && !gui.isPopupPanel(newView.Name()) {
			gui.onFocusLost(previousView, newView)
			gui.onFocus(newView)
			previousView = newView
		}
		return nil
	}
}

func (gui *Gui) onFocusChange() error {
	currentView := gui.g.CurrentView()
	for _, view := range gui.g.Views() {
		view.Highlight = view == currentView && gui.showsFocus(view)
	}
	return nil
}

func (gui *Gui) showsFocus(view *gocui.View) bool {
	return view.Name() != "main" || gui.qScreenActive()
}

func (gui *Gui) onFocusLost(v *gocui.View, newView *gocui.View) {
	if v == nil {
		return
	}

	if !gui.isPopupPanel(newView.Name()) {
		v.ParentView = nil
	}

	// A squashed resize can move the selected row outside the new viewport.
	gui.focusPointInView(v)

	gui.Log.Info(v.Name() + " focus lost")
}

func (gui *Gui) onFocus(v *gocui.View) {
	if v == nil {
		return
	}

	gui.focusPointInView(v)

	gui.Log.Info(v.Name() + " focus gained")
}
