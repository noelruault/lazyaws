// main_panel.go — the lazyaws port of lazydocker's pkg/gui/main_panel.go (MIT, © 2018 Jesse Duffield).
package ui

import (
	"github.com/jesseduffield/gocui"
)

func (gui *Gui) scrollUpMain() error {
	mainView := gui.Views.Main
	mainView.Autoscroll = false
	ox, oy := mainView.Origin()
	newOy := max(0, oy-gui.Config.User.Gui.ScrollHeight)
	return mainView.SetOrigin(ox, newOy)
}

func (gui *Gui) scrollDownMain() error {
	mainView := gui.Views.Main
	mainView.Autoscroll = false
	ox, oy := mainView.Origin()

	reservedLines := 0
	if !gui.Config.User.Gui.ScrollPastBottom {
		_, sizeY := mainView.Size()
		reservedLines = sizeY
	}

	totalLines := mainView.ViewLinesHeight()
	if oy+reservedLines >= totalLines {
		return nil
	}

	return mainView.SetOrigin(ox, oy+gui.Config.User.Gui.ScrollHeight)
}

func (gui *Gui) scrollLeftMain(g *gocui.Gui, v *gocui.View) error {
	mainView := gui.Views.Main
	ox, oy := mainView.Origin()
	newOx := max(0, ox-gui.Config.User.Gui.ScrollHeight)
	return mainView.SetOrigin(newOx, oy)
}

func (gui *Gui) scrollRightMain(g *gocui.Gui, v *gocui.View) error {
	mainView := gui.Views.Main
	ox, oy := mainView.Origin()

	content := mainView.ViewBufferLines()
	var largestNumberOfCharacters int
	for _, txt := range content {
		if len(txt) > largestNumberOfCharacters {
			largestNumberOfCharacters = len(txt)
		}
	}

	sizeX, _ := mainView.Size()
	if ox+sizeX >= largestNumberOfCharacters {
		return nil
	}

	return mainView.SetOrigin(ox+gui.Config.User.Gui.ScrollHeight, oy)
}

func (gui *Gui) autoScrollMain(g *gocui.Gui, v *gocui.View) error {
	gui.Views.Main.Autoscroll = true
	return nil
}

func (gui *Gui) jumpToTopMain(g *gocui.Gui, v *gocui.View) error {
	gui.Views.Main.Autoscroll = false
	_ = gui.Views.Main.SetOrigin(0, 0)
	_ = gui.Views.Main.SetCursor(0, 0)
	return nil
}

func (gui *Gui) onMainTabClick(tabIndex int) error {
	panel, ok := gui.sidePanelForMain()
	if !ok {
		return nil
	}

	panel.SetMainTabIndex(tabIndex)
	return panel.HandleSelect()
}

// handleMainNextTab resolves the backing panel so tab keys survive main focus.
func (gui *Gui) handleMainNextTab() error {
	panel, ok := gui.sidePanelForMain()
	if !ok {
		return nil
	}
	return panel.HandleNextMainTab()
}

func (gui *Gui) handleMainPrevTab() error {
	panel, ok := gui.sidePanelForMain()
	if !ok {
		return nil
	}
	return panel.HandlePrevMainTab()
}

func (gui *Gui) handleEnterMain(g *gocui.Gui, v *gocui.View) error {
	mainView := gui.Views.Main
	mainView.ParentView = v
	return gui.switchFocus(mainView)
}

func (gui *Gui) handleExitMain(g *gocui.Gui, v *gocui.View) error {
	v.ParentView = nil
	return gui.returnFocus()
}

func (gui *Gui) handleMainClick() error {
	if gui.popupPanelFocused() {
		return nil
	}

	currentView := gui.g.CurrentView()
	if currentView.Name() != "main" {
		gui.Views.Main.ParentView = currentView
	}

	return gui.switchFocus(gui.Views.Main)
}
