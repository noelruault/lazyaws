// Package ui routes profile refs, Enter, and menu actions through one switch path.
package ui

import (
	"context"
	"os/exec"

	"github.com/noelruault/lazyaws/ui/resources"
)

// ProfileActions keeps switching and signing in available in read-only mode because neither changes AWS state.
func (gui *Gui) ProfileActions() []resources.Action {
	profile, err := gui.Panels.Profile.GetSelectedItem()
	if err != nil {
		return nil
	}

	if profile != gui.CurrentProfile {
		return []resources.Action{{
			Name: "Switch to " + profile,
			Run: func(_ context.Context, _ string) error {
				// switchProfile runs its own waiting status and generation guard, so it is handed the profile rather than the action's context.
				return gui.switchProfile(profile)
			},
		}}
	}

	// The connected profile has nothing to switch to, so the only thing worth offering is the way out of a session it can no longer use.
	if gui.profileAuthProblem() == nil {
		return nil
	}

	return []resources.Action{{
		Name: loginCommand(profile),
		Run: func(_ context.Context, _ string) error {
			return gui.loginAndReconnect(profile)
		},
	}}
}

// loginAndReconnect hands the terminal to the AWS CLI and reconnects with whatever it wrote.
// An SSO login prints a code and waits on a browser, so it needs the tty rather than a captured pipe; runSubprocess is the same path ECS exec and the SSM session take.
// The reconnect is a full profile switch rather than a refresh: the credentials on disk are new, and the client holding the expired ones cannot be told about them.
func (gui *Gui) loginAndReconnect(profile string) error {
	if err := gui.runSubprocess(exec.Command("aws", "sso", "login", "--profile", profile)); err != nil {
		return err
	}

	return gui.switchProfile(profile)
}
