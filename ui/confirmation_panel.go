// confirmation_panel.go — the lazyaws port of lazydocker's pkg/gui/confirmation_panel.go (MIT, © 2018 Jesse Duffield; parts derive from the gocui examples, © 2014 The gocui Authors, BSD-style license).
package ui

import (
	"strings"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"
)

func (gui *Gui) wrappedConfirmationFunction(function func(*gocui.Gui, *gocui.View) error) func(*gocui.Gui, *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		if err := gui.closeConfirmationPrompt(); err != nil {
			return err
		}

		if function != nil {
			if err := function(g, v); err != nil {
				return err
			}
		}

		return nil
	}
}

func (gui *Gui) closeConfirmationPrompt() error {
	if err := gui.returnFocus(); err != nil {
		return err
	}
	gui.g.DeleteViewKeybindings("confirmation")
	gui.Views.Confirmation.Visible = false
	gui.Views.Confirmation.Editable = false
	gui.Views.Confirmation.FrameColor = gocui.ColorDefault
	gui.Views.Confirmation.ClearTextArea()
	return nil
}

// dismissPopups does not restore focus because screen swaps manage it themselves.
func (gui *Gui) dismissPopups() {
	if gui.Views.Confirmation != nil && gui.Views.Confirmation.Visible {
		gui.g.DeleteViewKeybindings("confirmation")
		gui.Views.Confirmation.Visible = false
		gui.Views.Confirmation.Editable = false
		gui.Views.Confirmation.FrameColor = gocui.ColorDefault
		gui.Views.Confirmation.ClearTextArea()
	}
	if gui.Views.Menu != nil {
		gui.Views.Menu.Visible = false
	}
}

func (gui *Gui) getMessageHeight(wrap bool, message string, width int) int {
	lines := strings.Split(message, "\n")
	lineCount := 0
	if wrap {
		for _, line := range lines {
			lineCount += len(line)/width + 1
		}
	} else {
		lineCount = len(lines)
	}
	return lineCount
}

const popupVerticalMargin = 4

func (gui *Gui) getConfirmationPanelDimensions(wrap bool, prompt string) (int, int, int, int) {
	width, height := gui.g.Size()
	panelWidth := width / 2
	panelHeight := gui.getMessageHeight(wrap, prompt, panelWidth)

	// Clamping keeps oversized menus scrollable instead of stranding off-screen items.
	if maxHeight := height - popupVerticalMargin; panelHeight > maxHeight {
		panelHeight = maxHeight
	}
	if panelHeight < 1 {
		panelHeight = 1
	}

	return width/2 - panelWidth/2,
		height/2 - panelHeight/2 - panelHeight%2 - 1,
		width/2 + panelWidth/2,
		height/2 + panelHeight/2
}

func (gui *Gui) createPromptPanel(title string, handleConfirm func(*gocui.Gui, *gocui.View) error) error {
	return gui.createPromptPanelWithFrame(title, gocui.ColorDefault, handleConfirm)
}

func (gui *Gui) createPromptPanelWithFrame(title string, frame gocui.Attribute, handleConfirm func(*gocui.Gui, *gocui.View) error) error {
	gui.onNewPopupPanel()
	err := gui.prepareConfirmationPanel(title, "")
	if err != nil {
		return err
	}
	gui.Views.Confirmation.Editable = true
	gui.Views.Confirmation.FrameColor = frame
	// The shared text area must be cleared between steps or the next prompt inherits stale input.
	gui.Views.Confirmation.ClearTextArea()
	return gui.setKeyBindings(gui.g, handleConfirm, nil)
}

func (gui *Gui) prepareConfirmationPanel(title, prompt string) error {
	x0, y0, x1, y1 := gui.getConfirmationPanelDimensions(true, prompt)
	confirmationView := gui.Views.Confirmation
	_, err := gui.g.SetView("confirmation", x0, y0, x1, y1, 0)
	if err != nil {
		return err
	}
	confirmationView.Title = title
	confirmationView.Visible = true
	gui.g.Update(func(g *gocui.Gui) error {
		return gui.switchFocus(confirmationView)
	})
	return nil
}

func (gui *Gui) onNewPopupPanel() {
	gui.Views.Menu.Visible = false
	gui.Views.Confirmation.Visible = false
}

// createConfirmationPanel never includes prompt text in errors because it may contain a secret.
// nolint:unparam
func (gui *Gui) createConfirmationPanel(title, prompt string, handleConfirm, handleClose func(*gocui.Gui, *gocui.View) error) error {
	return gui.createPopupPanel(title, prompt, handleConfirm, handleClose)
}

func (gui *Gui) createPopupPanel(title, prompt string, handleConfirm, handleClose func(*gocui.Gui, *gocui.View) error) error {
	// Actions call this from background work, so every view mutation must be queued through gocui.
	gui.g.Update(func(g *gocui.Gui) error {
		if gui.currentViewName() == "confirmation" {
			if err := gui.closeConfirmationPrompt(); err != nil {
				gui.Log.Error(err.Error())
			}
		}
		gui.onNewPopupPanel()
		err := gui.prepareConfirmationPanel(title, prompt)
		if err != nil {
			return err
		}
		gui.Views.Confirmation.Editable = false
		if err := gui.renderString(g, "confirmation", prompt); err != nil {
			return err
		}
		return gui.setKeyBindings(g, handleConfirm, handleClose)
	})
	return nil
}

func (gui *Gui) setKeyBindings(g *gocui.Gui, handleConfirm, handleClose func(*gocui.Gui, *gocui.View) error) error {
	if err := g.SetKeybinding("confirmation", gocui.KeyEnter, gocui.ModNone, gui.wrappedConfirmationFunction(handleConfirm)); err != nil {
		return err
	}
	if err := g.SetKeybinding("confirmation", 'y', gocui.ModNone, gui.wrappedConfirmationFunction(handleConfirm)); err != nil {
		return err
	}

	if err := g.SetKeybinding("confirmation", gocui.KeyEsc, gocui.ModNone, gui.wrappedConfirmationFunction(handleClose)); err != nil {
		return err
	}
	if err := g.SetKeybinding("confirmation", 'n', gocui.ModNone, gui.wrappedConfirmationFunction(handleClose)); err != nil {
		return err
	}

	return nil
}

func (gui *Gui) createErrorPanel(message string) error {
	colorFunction := color.New(color.FgRed).SprintFunc()
	coloredMessage := colorFunction(strings.TrimSpace(message))
	return gui.createConfirmationPanel("Error", coloredMessage, nil, nil)
}

func (gui *Gui) renderConfirmationOptions() error {
	optionsMap := map[string]string{
		"n/esc":   "no",
		"y/enter": "yes",
	}
	return gui.renderOptionsMap(optionsMap)
}
