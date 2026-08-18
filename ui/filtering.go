// Adapted from lazydocker's filtering flow (MIT, © 2018 Jesse Duffield).
package ui

import (
	"github.com/jesseduffield/gocui"
)

func (gui *Gui) handleOpenFilter() error {
	panel, ok := gui.currentListPanel()
	if !ok {
		return nil
	}

	if panel.IsFilterDisabled() {
		return nil
	}

	gui.State.Filter.active = true
	gui.State.Filter.panel = panel

	return gui.switchFocus(gui.Views.Filter)
}

func (gui *Gui) onNewFilterNeedle(value string) error {
	gui.State.Filter.needle = value
	gui.ResetOrigin(gui.State.Filter.panel.GetView())
	return gui.State.Filter.panel.RerenderList()
}

type editorFunc = func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool

func (gui *Gui) wrapEditor(f editorFunc) editorFunc {
	return gui.wrapEditorWith(f, gui.onNewFilterNeedle)
}

// wrapEditorWith runs change callbacks only after accepted edits so previews match visible input.
func (gui *Gui) wrapEditorWith(f editorFunc, onChange func(string) error) editorFunc {
	return func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
		matched := f(v, key, ch, mod)
		if matched {
			if err := onChange(v.TextArea.GetContent()); err != nil {
				gui.Log.Error(err.Error())
			}
		}
		return matched
	}
}

func (gui *Gui) escapeFilterPrompt() error {
	if err := gui.clearFilter(); err != nil {
		return err
	}

	return gui.returnFocus()
}

func (gui *Gui) clearFilter() error {
	gui.State.Filter.needle = ""
	gui.State.Filter.active = false
	panel := gui.State.Filter.panel
	gui.State.Filter.panel = nil
	gui.Views.Filter.ClearTextArea()

	if panel == nil {
		return nil
	}

	gui.ResetOrigin(panel.GetView())

	return panel.RerenderList()
}

func (gui *Gui) commitFilter() error {
	if gui.State.Filter.needle == "" {
		if err := gui.clearFilter(); err != nil {
			return err
		}
	}

	return gui.returnFocus()
}

func (gui *Gui) filterPrompt() string {
	return "filter: "
}

func (gui *Gui) FilterString(view *gocui.View) string {
	if gui.State.Filter.panel != nil && gui.State.Filter.panel.GetView() != view {
		return ""
	}

	return gui.State.Filter.needle
}
