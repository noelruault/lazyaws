package ui

import (
	"slices"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/ui/panels"
)

// sideViewNames excludes hidden panels from keyboard navigation and layout ordering.
func (gui *Gui) sideViewNames() []string {
	visible := make([]panels.ISideListPanel, 0, len(gui.allSidePanels()))
	for _, panel := range gui.allSidePanels() {
		if !panel.IsHidden() {
			visible = append(visible, panel)
		}
	}

	return sidePanelViewNames(visible)
}

func sidePanelViewNames(sidePanels []panels.ISideListPanel) []string {
	names := make([]string, len(sidePanels))
	for i, panel := range sidePanels {
		names[i] = panel.GetView().Name()
	}

	return names
}

// resourceViewNames are the views that address the selected resource: the eight lists, plus main, which shows that same resource's detail.
// Keys that act on a selection are registered from this list rather than globally, so they cannot fire in the chat, a filter prompt or a popup.
func resourceViewNames(sidePanels []panels.ISideListPanel) []string {
	return append(sidePanelViewNames(sidePanels), "main")
}

func (gui *Gui) popupViewNames() []string {
	return []string{"confirmation", "menu"}
}

func (gui *Gui) initiallyFocusedViewName() string {
	return "profile"
}

func (gui *Gui) isPopupPanel(viewName string) bool {
	return slices.Contains(gui.popupViewNames(), viewName)
}

func (gui *Gui) popupPanelFocused() bool {
	return gui.isPopupPanel(gui.currentViewName())
}

func (gui *Gui) newLineFocused(v *gocui.View) error {
	if v == nil {
		return nil
	}

	currentListPanel, ok := gui.currentListPanel()
	if ok {
		return currentListPanel.HandleSelect()
	}

	switch v.Name() {
	case "confirmation":
		return nil
	case "main":
		v.Highlight = false
		return nil
	case "filter", "command", "qInput", "qChats", "settings":
		return nil
	default:
		panic("No view matching newLineFocused switch statement")
	}
}

func (gui *Gui) switchFocus(newView *gocui.View) error {
	gui.Mutexes.ViewStackMutex.Lock()
	defer gui.Mutexes.ViewStackMutex.Unlock()

	return gui.switchFocusAux(newView)
}

func (gui *Gui) switchFocusAux(newView *gocui.View) error {
	gui.pushView(newView.Name())
	gui.Log.Info("setting highlight to true for view " + newView.Name())
	gui.Log.Info("new focused view is " + newView.Name())
	if _, err := gui.g.SetCurrentView(newView.Name()); err != nil {
		return err
	}

	gui.g.Cursor = newView.Editable

	if err := gui.renderPanelOptions(); err != nil {
		return err
	}

	newViewStack := gui.State.ViewStack

	if gui.State.Filter.panel != nil && !slices.Contains(newViewStack, gui.State.Filter.panel.GetView().Name()) {
		if err := gui.clearFilter(); err != nil {
			return err
		}
	}

	if !slices.Contains(newViewStack, "menu") {
		gui.Views.Menu.Visible = false
	}

	return gui.newLineFocused(newView)
}

func (gui *Gui) returnFocus() error {
	gui.Mutexes.ViewStackMutex.Lock()
	defer gui.Mutexes.ViewStackMutex.Unlock()

	if len(gui.State.ViewStack) <= 1 {
		return nil
	}

	previousViewName := gui.State.ViewStack[len(gui.State.ViewStack)-2]
	previousView, err := gui.g.View(previousViewName)
	if err != nil {
		return err
	}

	return gui.switchFocusAux(previousView)
}

// pushView must remain behind switchFocus so stack and gocui focus change atomically.
func (gui *Gui) pushView(name string) {
	// Filters keep popups in the stack because the menu itself can be searched.
	if name != "filter" {
		gui.State.ViewStack = slices.DeleteFunc(gui.State.ViewStack, gui.isPopupPanel)
	}

	if slices.Contains(gui.sideViewNames(), name) {
		gui.State.ViewStack = []string{}
	}

	gui.State.ViewStack = slices.DeleteFunc(gui.State.ViewStack, func(viewName string) bool {
		return viewName == name
	})

	gui.State.ViewStack = append(gui.State.ViewStack, name)
}

func (gui *Gui) removeViewFromStack(view *gocui.View) {
	gui.Mutexes.ViewStackMutex.Lock()
	defer gui.Mutexes.ViewStackMutex.Unlock()

	gui.State.ViewStack = slices.DeleteFunc(gui.State.ViewStack, func(viewName string) bool {
		return viewName == view.Name()
	})
}

func (gui *Gui) currentStaticViewName() string {
	gui.Mutexes.ViewStackMutex.Lock()
	defer gui.Mutexes.ViewStackMutex.Unlock()

	for i := len(gui.State.ViewStack) - 1; i >= 0; i-- {
		if !slices.Contains(gui.popupViewNames(), gui.State.ViewStack[i]) {
			return gui.State.ViewStack[i]
		}
	}

	return gui.initiallyFocusedViewName()
}

func (gui *Gui) currentSideViewName() string {
	gui.Mutexes.ViewStackMutex.Lock()
	defer gui.Mutexes.ViewStackMutex.Unlock()

	for idx := range gui.State.ViewStack {
		reversedIdx := len(gui.State.ViewStack) - 1 - idx
		viewName := gui.State.ViewStack[reversedIdx]
		if slices.Contains(gui.sideViewNames(), viewName) {
			return viewName
		}
	}

	return gui.initiallyFocusedViewName()
}
