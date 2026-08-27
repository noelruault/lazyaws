package ui

const copyPopupTitle = "id / ARN (select to copy)"

// handleCopySelected shows the selected row's identifier in a popup instead of writing it to the clipboard: the pinned gocui exposes no clipboard API, and this is the same no-dependency answer the presigned-URL and console-URL actions already give.
// It resolves the panel through focus history, so it answers for the resource whose detail pane is open even once focus has moved into main.
func (gui *Gui) handleCopySelected() error {
	// The chat screen can put focus on main (qChats hands it there), and the lists are hidden behind it, so the value focus history would answer with belongs to a resource that is not on screen.
	if gui.mainBelongsToQ() {
		return nil
	}

	panel, ok := gui.sidePanelForMain()
	if !ok {
		return nil
	}

	value, ok := panel.SelectedCopyValue()
	if !ok {
		return nil
	}

	return gui.createConfirmationPanel(copyPopupTitle, value, nil, nil)
}

// arnOrName prefers the ARN because that is what pastes into a policy or a CLI call, and falls back to the name for a row whose list call answered without one.
func arnOrName(arn, name string) string {
	if arn != "" {
		return arn
	}

	return name
}
