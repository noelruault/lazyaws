package ui

import (
	"errors"
	"strings"
	"testing"

	awsapp "github.com/noelruault/lazyaws/apps/aws"
)

func TestAuthFailureMessage(t *testing.T) {
	// Captured from an expired AWS SSO session.
	cause := errors.New("failed to list EKS clusters: operation error EKS: ListClusters, get identity: get credentials: failed to refresh cached SSO token, operation error SSO OIDC: CreateToken, https response error StatusCode: 400, RequestID: 1792aad3-b057-424c-b4b0-f091232ca8ae, InvalidGrantException")

	message := authFailureMessage("prod", cause)

	for _, want := range []string{
		"can't use your AWS credentials",
		"profile: prod",
		"aws sso login --profile prod",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("message = %q, want it to contain %q", message, want)
		}
	}

	if !strings.Contains(message, "InvalidGrantException") {
		t.Errorf("message = %q, want the underlying reason kept", message)
	}
	if !strings.Contains(message, "your SSO session has expired") {
		t.Errorf("message = %q, want the expired session said plainly", message)
	}
}

func TestLoginProblem(t *testing.T) {
	tests := []struct {
		raw     string
		reason  string
		isLogin bool
	}{
		{"get credentials: failed to refresh cached SSO token, operation error SSO OIDC: CreateToken", "your SSO session has expired", true},
		{"https response error StatusCode: 400, InvalidGrantException", "your SSO session has expired", true},
		{"operation error STS: GetCallerIdentity, ExpiredToken: The security token included in the request is expired", "your SSO session has expired", true},
		{"failed to retrieve credentials from IMDS", "no credentials found for this profile", true},
		{"operation error STS: GetCallerIdentity, AccessDenied", "", false},
		{"no AWS region configured: set region in ~/.aws/config", "", false},
		{"dial tcp: lookup sts.eu-west-1.amazonaws.com: no such host", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		reason, isLogin := loginProblem(tt.raw)
		if reason != tt.reason || isLogin != tt.isLogin {
			t.Errorf("loginProblem(%q) = (%q, %v), want (%q, %v)", tt.raw, reason, isLogin, tt.reason, tt.isLogin)
		}
	}
}

func TestAuthFailureMessageWithoutAProfile(t *testing.T) {
	message := authFailureMessage("", errors.New("no AWS credentials found"))

	if strings.Contains(message, "--profile") {
		t.Errorf("message = %q, want no empty --profile flag", message)
	}
	if !strings.Contains(message, "aws sso login") {
		t.Errorf("message = %q, want the plain login command", message)
	}
	if strings.Contains(message, "profile:") {
		t.Errorf("message = %q, want no empty profile line", message)
	}
}

func TestPreflightExitsWhenThereIsNothingToSwitchTo(t *testing.T) {
	degraded, err := preflight(nil, "prod", nil)

	if degraded {
		t.Error("preflight() wants to start the UI with no profiles to offer")
	}
	if err == nil {
		t.Fatal("preflight() error = nil, want a refusal")
	}
	if !IsStartupFailure(err) {
		t.Errorf("IsStartupFailure() = false for %T, want the failure to be recognisable", err)
	}
	if !strings.Contains(err.Error(), "aws sso login --profile prod") {
		t.Errorf("error = %q, want the login command", err)
	}
}

// Alternative profiles keep authentication failure recoverable inside the app.
func TestPreflightStartsAnywayWhenProfilesExist(t *testing.T) {
	degraded, err := preflight(nil, "prod", []string{"prod", "staging"})

	if err != nil {
		t.Fatalf("preflight() error = %v, want it to start in degraded mode", err)
	}
	if !degraded {
		t.Error("preflight() = false, want degraded mode so the problem gets shown in the UI")
	}
}

func TestPreflightWithWorkingCredentials(t *testing.T) {
	degraded, err := preflight(&awsapp.Client{}, "prod", []string{"prod"})

	if err != nil || degraded {
		t.Errorf("preflight() = (%v, %v), want a normal start", degraded, err)
	}
}

func TestDegradedModeMessage(t *testing.T) {
	message := degradedModeMessage("prod", errors.New("operation error STS: GetCallerIdentity, get identity: get credentials: failed to refresh cached SSO token, InvalidGrantException"))

	for _, want := range []string{
		"prod",
		"your SSO session has expired",
		"InvalidGrantException",
		"Profiles panel",
		"aws sso login --profile prod",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("message = %q, want it to contain %q", message, want)
		}
	}
}

func TestIsStartupFailure(t *testing.T) {
	if IsStartupFailure(errors.New("some other problem")) {
		t.Error("IsStartupFailure() = true for an unrelated error")
	}
	if IsStartupFailure(nil) {
		t.Error("IsStartupFailure(nil) = true")
	}

	wrapped := errors.New("wrapped: " + (&startupError{message: "x"}).Error())
	if IsStartupFailure(wrapped) {
		t.Error("IsStartupFailure() = true for an error that merely quotes the message")
	}
}

// Multi-line SDK causes must survive startup formatting intact.
func TestTheWholeErrorIsPrinted(t *testing.T) {
	cause := errors.New("first line, the useful one\nsecond line, also the user's business\nthird line")

	message := authFailureMessage("prod", cause)

	for _, want := range []string{"first line, the useful one", "second line, also the user's business", "third line"} {
		if !strings.Contains(message, want) {
			t.Errorf("message = %q, dropped %q", message, want)
		}
	}
}

func TestNoLoginAdviceForUnrelatedFailures(t *testing.T) {
	message := authFailureMessage("prod", errors.New("dial tcp: lookup sts.eu-west-1.amazonaws.com: no such host"))

	if strings.Contains(message, "aws sso login") {
		t.Errorf("message = %q, want no login advice for a DNS failure", message)
	}
	if !strings.Contains(message, "no such host") {
		t.Errorf("message = %q, want the real error", message)
	}
	if strings.Contains(message, "reason:") {
		t.Errorf("message = %q, want no invented reason", message)
	}
}

// Missing default selection needs profile guidance, not login guidance.
func TestClientFailureNamesTheProfilesYouHave(t *testing.T) {
	cause := errors.New("no AWS region configured: set region in ~/.aws/config, AWS_REGION, or -region")
	profiles := []string{"staging", "prod", "cicd"}

	message := clientFailureMessage("", cause, profiles)

	if !strings.Contains(message, "no AWS region configured") {
		t.Errorf("message = %q, want the real error", message)
	}
	if !strings.Contains(message, "AWS_PROFILE=staging lazyaws") {
		t.Errorf("message = %q, want a runnable suggestion", message)
	}
	if !strings.Contains(message, "cicd") {
		t.Errorf("message = %q, want the available profiles listed", message)
	}
	if strings.Contains(message, "aws sso login") {
		t.Errorf("message = %q, want no login advice: logging in doesn't set a region", message)
	}
}

func TestClientFailureWithNoProfilesAtAll(t *testing.T) {
	message := clientFailureMessage("", errors.New("no AWS region configured"), nil)

	if !strings.Contains(message, "No profiles found") {
		t.Errorf("message = %q, want it to say there is nothing to pick from", message)
	}
}

func TestClientFailureWithAProfileSet(t *testing.T) {
	message := clientFailureMessage("staging", errors.New("failed to load config"), []string{"staging"})

	if !strings.Contains(message, "profile: staging") {
		t.Errorf("message = %q, want the profile named", message)
	}
	if strings.Contains(message, "AWS_PROFILE=") {
		t.Errorf("message = %q, want no advice to set a profile that is already set", message)
	}
}

// Degraded startup must keep recovery usable without launching doomed AWS loaders.
func TestDegradedStartShowsTheProblemOnceAndKeepsProfilesUsable(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, func() error {
		gui.CurrentProfile = "prod"
		gui.authProblem = errors.New("operation error STS: GetCallerIdentity, get credentials: failed to refresh cached SSO token, InvalidGrantException")
		gui.refresh()
		return nil
	})

	main := waitForView(t, g, gui.Views.Main, "Can't reach AWS with these credentials")
	if !strings.Contains(main, "your SSO session has expired") {
		t.Errorf("main = %q, want the reason", main)
	}
	if !strings.Contains(main, "Profiles panel") {
		t.Errorf("main = %q, want the way out named", main)
	}
	if got := strings.Count(main, "Can't reach AWS"); got != 1 {
		t.Errorf("the problem is reported %d times, want once", got)
	}

	if !ask(g, func() bool { return gui.panelReloaders()["profile"] != nil }) {
		t.Error("no profile reloader to run in degraded mode")
	}
}

func TestAWorkingProfileSwitchClearsTheProblem(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, func() error {
		gui.authProblem = errors.New("expired")
		return gui.applyProfileSwitch(gui.Gen, "staging", &awsapp.Client{})
	})

	if ask(g, func() error { return gui.authProblem }) != nil {
		t.Error("authProblem survived a working profile switch")
	}
	if got := ask(g, func() string { return gui.CurrentProfile }); got != "staging" {
		t.Errorf("CurrentProfile = %q, want the switched-to profile", got)
	}
}

// Popup height must remain bounded so every item is reachable by scrolling.
func TestPopupsFitTheScreen(t *testing.T) {
	gui, g := newHeadlessGui(t)

	var content strings.Builder
	for i := 0; i < 120; i++ {
		content.WriteString("global.anthropic.claude-sonnet-4-6\n")
	}

	box := ask(g, func() [4]int {
		a, b, c, d := gui.getConfirmationPanelDimensions(false, content.String())
		return [4]int{a, b, c, d}
	})
	y0, y1 := box[1], box[3]

	screenHeight := ask(g, func() int {
		_, h := g.Size()
		return h
	})

	if y0 < 0 {
		t.Errorf("popup starts at y=%d, above the top of the screen", y0)
	}
	if y1 > screenHeight {
		t.Errorf("popup ends at y=%d, past the bottom of the %d-row screen", y1, screenHeight)
	}
	if y1-y0 < 3 {
		t.Errorf("popup is %d rows tall, too small to show a list", y1-y0)
	}
}
