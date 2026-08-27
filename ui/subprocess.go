// Portions adapted from lazydocker's subprocess helper (MIT, © 2018 Jesse Duffield).
// Suspending gocui and pausing refreshes are one invariant so child processes own the tty exclusively.
package ui

import (
	"io"
	"os"
	"os/exec"
)

// runSubprocess must stay off the UI thread because interactive children can block indefinitely.
func (gui *Gui) runSubprocess(cmd *exec.Cmd) error {
	gui.Mutexes.SubprocessMutex.Lock()
	defer gui.Mutexes.SubprocessMutex.Unlock()

	if err := gui.g.Suspend(); err != nil {
		return err
	}
	gui.PauseBackgroundThreads.Store(true)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	runErr := cmd.Run()
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	gui.PauseBackgroundThreads.Store(false)
	if err := gui.g.Resume(); err != nil {
		return err
	}
	// The child owned the tty and almost certainly scrolled it, so tcell's cell model no longer describes the screen and a differential redraw would leave the child's output stranded.
	forceRepaint()

	return runErr
}
