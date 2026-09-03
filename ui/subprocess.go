// Suspending gocui and pausing refreshes are one invariant so child processes own the tty exclusively.
package ui

import (
	"io"
	"os"
	"os/exec"
)

// runSubprocess hands the terminal to a child that changes AWS state, so it refuses while writes are denied.
// This is a second gate rather than a duplicate one: the shells here are `aws ecs execute-command` and `aws ssm start-session`, child processes carrying their own SDK, so the read-only guard on our clients never sees them and could not refuse them.
// It must stay off the UI thread because interactive children can block indefinitely.
func (gui *Gui) runSubprocess(cmd *exec.Cmd) error {
	if gui.readOnly() {
		return gui.refuseReadOnly("Running " + subprocessName(cmd))
	}

	return gui.runSubprocessAllowedInReadOnly(cmd)
}

// runSubprocessAllowedInReadOnly is for the children that touch nothing in AWS: an editor on the local config file, and signing in.
// Named for what it permits rather than what it does, so a new caller has to argue for the exemption instead of inheriting it by picking the shorter function.
func (gui *Gui) runSubprocessAllowedInReadOnly(cmd *exec.Cmd) error {
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

// subprocessName is what the refusal calls the child, so the message names the command a user recognises rather than a path.
func subprocessName(cmd *exec.Cmd) string {
	if cmd == nil || cmd.Path == "" {
		return "that command"
	}

	if len(cmd.Args) > 1 {
		return cmd.Args[0] + " " + cmd.Args[1]
	}

	return cmd.Args[0]
}
