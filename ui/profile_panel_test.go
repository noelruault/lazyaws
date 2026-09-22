package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/ui/utils"
)

// Profile section matching must not leak neighboring configuration.
func TestReadAWSConfigSection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[default]\nregion = us-east-1\n\n[profile staging]\nregion = eu-west-1\nrole_arn = arn:aws:iam::123:role/staging\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		profile string
		wantSub string
	}{
		{"default", "region = us-east-1"},
		{"staging", "role_arn = arn:aws:iam::123:role/staging"},
		{"missing", "no config section found for profile missing"},
	}

	for _, tt := range tests {
		got := readAWSConfigSection(tt.profile)
		if !strings.Contains(got, tt.wantSub) {
			t.Errorf("readAWSConfigSection(%q) = %q, want substring %q", tt.profile, got, tt.wantSub)
		}
	}

	if strings.Contains(readAWSConfigSection("default"), "eu-west-1") {
		t.Error("readAWSConfigSection(\"default\") leaked staging's region")
	}
}

// refreshProfile is a reloader: it runs on r/R and on the background refresh, not only at startup.
// It opens the panel on the connected profile, but once the cursor has moved a later refresh must leave it where the user put it.
func TestProfileRefreshOpensOnTheCurrentProfileThenLeavesTheCursorAlone(t *testing.T) {
	writeAWSProfiles(t, "alpha", "staging", "zeta")

	gui, g := newHeadlessGui(t)
	gui.CurrentProfile = "staging"

	reloadProfiles(t, g, gui, 3)
	if got := ask(g, func() int { return gui.Panels.Profile.SelectedIdx }); got != 1 {
		t.Fatalf("SelectedIdx after the first load = %d, want 1 (the connected profile)", got)
	}

	run(t, g, func() error {
		gui.Panels.Profile.SetSelectedLineIdx(2)
		return nil
	})

	// A fourth profile is what makes the second reload observable, so the assertion below is about a list that really was replaced.
	writeAWSProfiles(t, "alpha", "staging", "zeta", "zzz")
	reloadProfiles(t, g, gui, 4)
	if got := ask(g, func() int { return gui.Panels.Profile.SelectedIdx }); got != 2 {
		t.Errorf("SelectedIdx after a refresh = %d, want 2 (the row the cursor was moved to, not the connected profile)", got)
	}
}

// The loop renders the list while a reload replaces it, so the swap has to happen ON the loop; -race is what proves it, not an assertion.
func TestReloadingTheProfilePanelWhileItRerendersIsRaceFree(t *testing.T) {
	writeAWSProfiles(t, "alpha", "staging")

	gui, g := newHeadlessGui(t)
	gui.CurrentProfile = "staging"

	// Off the loop, which is where the refresh tier and the panel throttle both call this loader from.
	reloaded := make(chan struct{})
	go func() {
		defer close(reloaded)
		for range 50 {
			if err := gui.refreshProfile(); err != nil {
				t.Errorf("refreshProfile() = %v", err)

				return
			}
		}
	}()

	for range 50 {
		run(t, g, func() error { return gui.Panels.Profile.RerenderList() })
	}
	<-reloaded
}

// writeAWSProfiles points HOME at a config holding exactly these profiles, which is the file listAWSProfiles reads.
func writeAWSProfiles(t *testing.T, profiles ...string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".aws"), 0o755); err != nil {
		t.Fatal(err)
	}

	var config strings.Builder
	for _, profile := range profiles {
		config.WriteString("[profile " + profile + "]\n")
	}
	if err := os.WriteFile(filepath.Join(home, ".aws", "config"), []byte(config.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// reloadProfiles runs the loader the way the refresh tier does, off the loop, and waits for the swap it queues onto the loop to land.
func reloadProfiles(t *testing.T, g *gocui.Gui, gui *Gui, want int) {
	t.Helper()

	if err := gui.refreshProfile(); err != nil {
		t.Fatalf("refreshProfile() = %v", err)
	}

	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if ask(g, func() int { return gui.Panels.Profile.List.Len() }) == want {
			return
		}
	}

	t.Fatalf("the profile list never reached %d rows", want)
}

// fakeAWSOnPath puts a stand-in for the AWS CLI first on PATH, which is how the login path is driven without a browser.
func fakeAWSOnPath(t *testing.T, body string) {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "aws"), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("writing fake aws: %v", err)
	}
	t.Setenv("PATH", dir)
}

// The login prints a URL and waits on a browser. Handing it the terminal is what left the app drawing into a screen that was gone, so it runs with its output piped and that output has to reach the pane, or the URL exists nowhere the user can see when the browser fails to open.
func TestTheSSOLoginShowsItsURLWithoutTakingTheTerminal(t *testing.T) {
	url := "https://device.sso.eu-west-1.amazonaws.com/?user_code=ABCD-1234"
	fakeAWSOnPath(t, "printf '%s\\n' 'Attempting to open your default browser.' '"+url+"'")

	gui, g := newHeadlessGui(t)

	transcript := ask(g, func() string {
		out, err := gui.runSSOLogin("prod")
		if err != nil {
			t.Errorf("the login reported %v", err)
		}
		return out
	})

	if !strings.Contains(transcript, url) {
		t.Errorf("the transcript lost the URL:\n%s", transcript)
	}
	if pane := waitForView(t, g, gui.Views.Main, url); !strings.Contains(pane, "Signing in to prod") {
		t.Errorf("the pane does not say what is happening:\n%s", pane)
	}
}

// A failed login has to keep the CLI's own words: "exit status 1" alone tells the user nothing about which of the many ways it went wrong.
func TestAFailedSSOLoginKeepsWhatTheCLISaid(t *testing.T) {
	fakeAWSOnPath(t, "echo 'The SSO session associated with this profile has expired' >&2; exit 1")

	gui, g := newHeadlessGui(t)

	transcript := ask(g, func() string {
		out, err := gui.runSSOLogin("prod")
		if err == nil {
			t.Error("a failing aws sso login was reported as success")
		}
		return out
	})

	if !strings.Contains(transcript, "has expired") {
		t.Errorf("the transcript lost the CLI's reason:\n%s", transcript)
	}
}

// An expired session used to reach this pane as "Account ID: none", which names no cause and offers no way out.
// The startup banner does say both, but the first render of this tab paints over it, so the words have to live here too.
func TestTheCredentialsTabSaysHowToSignInWhenTheSessionIsGone(t *testing.T) {
	expired := errors.New("operation error SSO: GetRoleCredentials, failed to refresh cached SSO token: expired")

	pane := signInMessage("prod", expired)

	for _, want := range []string{
		"Not signed in",
		"your SSO session has expired",
		"aws sso login --profile prod",
	} {
		if !strings.Contains(utils.Decolorise(pane), want) {
			t.Errorf("the pane does not say %q:\n%s", want, pane)
		}
	}

	// A cause the matcher does not recognise still has to produce the command, because the command is the part the user acts on.
	unknown := signInMessage("prod", errors.New("something nobody has seen before"))
	if !strings.Contains(utils.Decolorise(unknown), "aws sso login --profile prod") {
		t.Errorf("an unrecognised cause loses the login command:\n%s", unknown)
	}
}

// Away from the profile panel the panel collapses to one row, and the other views read that row as the account they are showing.
// A cursor left on a profile the user only scrolled past would name an account the resources did not come from, so leaving the panel for another dashboard view snaps it back.
func TestLeavingTheProfilePanelSnapsTheCursorBackToTheConnectedProfile(t *testing.T) {
	writeAWSProfiles(t, "prod", "stage")

	gui, g := newHeadlessGui(t)
	gui.CurrentProfile = "prod"
	reloadProfiles(t, g, gui, 2)

	run(t, g, func() error {
		gui.Panels.Profile.SelectByItem("stage")
		gui.onFocusLost(gui.Views.Profile, gui.Views.ECS)
		return nil
	})
	if got := ask(g, func() int { return gui.Panels.Profile.SelectedIdx }); got != 0 {
		t.Errorf("SelectedIdx after moving to ECS = %d, want 0 (prod, the connected profile)", got)
	}

	// The action menu acts on the row the cursor is pointing at, so opening it must leave that row selected.
	run(t, g, func() error {
		gui.Panels.Profile.SelectByItem("stage")
		gui.onFocusLost(gui.Views.Profile, gui.Views.Menu)
		return nil
	})
	if got := ask(g, func() int { return gui.Panels.Profile.SelectedIdx }); got != 1 {
		t.Errorf("SelectedIdx after opening the menu = %d, want 1 (stage, the row the menu acts on)", got)
	}
}
