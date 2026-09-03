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
		// Permitted while read-only: the file is lazyaws's own settings on this machine, and nothing in AWS moves because someone edited it.
		return gui.runSubprocessAllowedInReadOnly(exec.Command(editor, config.ConfigFilename()))
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
