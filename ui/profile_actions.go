// Package ui routes profile refs, Enter, and menu actions through one switch path.
package ui

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"strings"
	"time"

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

// ssoLoginTimeout is how long the browser half of the login gets before the attempt is abandoned.
// Generous on purpose: the wait includes finding the window, signing in and approving, and the cost of being wrong is a login the user has to start again.
const ssoLoginTimeout = 3 * time.Minute

// loginAndReconnect must NOT suspend gocui, unlike ECS exec and the SSM session: this child wants no keyboard, and suspending for it left the app rendering into a screen that was gone, so the terminal came back cooked and echoing.
// The transcript reaches main as it arrives so the URL is on screen when the browser does not open by itself.
func (gui *Gui) loginAndReconnect(profile string) error {
	transcript, err := gui.runSSOLogin(profile)
	if err != nil {
		return gui.createErrorPanel(loginCommand(profile) + " failed:\n\n" + strings.TrimSpace(transcript) + "\n\n" + err.Error())
	}

	return gui.switchProfile(profile)
}

// runSSOLogin hands back the transcript as well as the error so a failure can be shown with the CLI's own words, and so a test can drive the whole path with a fake aws on PATH instead of a browser.
func (gui *Gui) runSSOLogin(profile string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ssoLoginTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "aws", "sso", "login", "--profile", profile)

	reader, writer := io.Pipe()
	cmd.Stdout, cmd.Stderr = writer, writer

	done := make(chan error, 1)
	go func() {
		err := cmd.Run()
		// Closing the writer is what ends the scan below, so it has to happen whether or not the command worked.
		_ = writer.Close()
		done <- err
	}()

	var transcript strings.Builder
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		transcript.WriteString(scanner.Text() + "\n")
		gui.reRenderStringMainOrdered(loginTranscript(profile, transcript.String()))
	}

	return transcript.String(), <-done
}

// loginTranscript frames the CLI's own words rather than replacing them, because the URL and the device code it prints are the whole point of showing this.
func loginTranscript(profile string, output string) string {
	return "Signing in to " + profile + ", finish it in your browser.\n\n" + output
}
