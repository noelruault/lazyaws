package ui

import (
	"github.com/jesseduffield/gocui"
)

func (gui *Gui) layout(g *gocui.Gui) error {
	g.Highlight = true
	width, height := g.Size()

	appStatus := gui.statusManager.getStatusString()
	informationStr := gui.getInformationContent()

	viewDimensions := gui.getWindowDimensions(informationStr, appStatus)

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

	// The information cell is painted here rather than from a render pass because layout is the one place that already knows the string, and the box was sized from that same value a few lines up: writing it anywhere else lets the text and the width it reserved drift apart.
	// The padding leads the version because the version ends the line: it is the gap after whatever precedes it, and it is the space the box was sized for.
	if err := gui.setViewContent(gui.Views.Information, infoSectionPadding+informationStr); err != nil {
		return err
	}

	// Chat rewraps here because layout owns race-free access to the new width.
	gui.syncQWidth()

	// Width-aware tabs re-render here for the same reason, and after the loop above so main already carries its new size.
	gui.syncMainWidth()

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
		focused := view == currentView && gui.showsFocus(view)

		// A resource list marks its selected row whether or not it holds focus: the main pane describes that row, and drilling into it moves focus away, so a list that marks its selection only while focused leaves the pane describing a resource nothing on screen points at.
		// Only the focused list gets the selection bar; the rest keep the bold, brightened row gocui draws under Highlight when no background is set, so eight lists never claim the cursor at once.
		if _, isList := gui.sidePanelNamed(view.Name()); isList {
			view.Highlight = true
			view.SelBgColor = gocui.ColorDefault
			if focused {
				view.SelBgColor = gui.selectedLineBgColor
			}
			continue
		}

		view.Highlight = focused
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

	if v.Name() == "profile" {
		gui.snapProfileToConnected(newView.Name())
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
