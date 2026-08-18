// Adapted from lazydocker's edit-config flow (MIT, © 2018 Jesse Duffield).
package ui

import (
	"os"
	"os/exec"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/config"
)

func (gui *Gui) handleOpenConfig(g *gocui.Gui, v *gocui.View) error {
	editor := configEditor()
	if editor == "" {
		return gui.createErrorPanel("no editor defined in $VISUAL or $EDITOR")
	}

	return gui.WithWaitingStatus("opening editor", func() error {
		return gui.runSubprocess(exec.Command(editor, config.ConfigFilename()))
	})
}

func configEditor() string {
	if editor := os.Getenv("VISUAL"); editor != "" {
		return editor
	}
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	if _, err := exec.LookPath("vi"); err == nil {
		return "vi"
	}
	return ""
}
