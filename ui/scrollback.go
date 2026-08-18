package ui

import (
	"io"
	"os"

	"github.com/jesseduffield/gocui"
)

// clearScrollback (CSI 3J) drops the terminal's saved lines. gocui does not own the scrollback, so without this the session can still be scrolled back into whatever preceded it.
const clearScrollback = "\x1b[3J"

// blockScrollback is a no-op off a terminal so piped output stays free of escapes.
func blockScrollback(stdout *os.File) {
	info, err := stdout.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return
	}

	_, _ = io.WriteString(stdout, clearScrollback)
}

// forceRepaint discards tcell's differential model and repaints every cell.
// A terminal scroll leaves that model describing rows the terminal no longer holds, and every
// later frame then skips cells it believes are already correct, so the tearing never heals on its own.
func forceRepaint() {
	if gocui.Screen != nil {
		gocui.Screen.Sync()
	}
}

func (gui *Gui) handleRedraw() error {
	forceRepaint()

	return nil
}
