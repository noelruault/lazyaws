package ui

import (
	"errors"
	"os"
	"strings"

	awsapp "github.com/noelruault/lazyaws/apps/aws"
)

type startupError struct {
	message string
}

func (e *startupError) Error() string { return e.message }

// preflight starts degraded when profile switching can recover; otherwise it fails before gocui owns the terminal.
func preflight(client *awsapp.Client, profile string, profiles []string) (startInDegradedMode bool, err error) {
	cause := error(nil)
	switch {
	case client == nil:
		cause = errors.New("no AWS credentials found")
	default:
		cause = client.AuthError()
	}
	if cause == nil {
		return false, nil
	}

	if len(profiles) > 0 {
		return true, nil
	}

	return false, &startupError{message: authFailureMessage(profile, cause)}
}

func credentialsProblem(client *awsapp.Client, clientErr error) error {
	if clientErr != nil {
		return clientErr
	}
	if client == nil {
		return errors.New("no AWS credentials found")
	}

	return client.AuthError()
}

func degradedModeMessage(profile string, cause error) string {
	var out strings.Builder

	out.WriteString("Can't reach AWS with these credentials.\n\n")
	if profile != "" {
		out.WriteString("  profile: " + profile + "\n")
	}
	if reason, _ := loginProblem(strings.TrimSpace(cause.Error())); reason != "" {
		out.WriteString("  reason:  " + reason + "\n")
	}
	out.WriteString("\n" + indent(strings.TrimSpace(cause.Error())) + "\n")
	out.WriteString("\nPick another profile in the Profiles panel (enter switches to it), or quit, ")
	out.WriteString(loginCommand(profile))
	out.WriteString(" and start again.\n")

	return out.String()
}

// authFailureMessage adds login advice only for recognized login failures while preserving every cause.
func authFailureMessage(profile string, cause error) string {
	var out strings.Builder

	raw := strings.TrimSpace(cause.Error())
	reason, isLoginProblem := loginProblem(raw)

	out.WriteString("lazyaws can't use your AWS credentials, so there's nothing it could show you.\n\n")
	if profile != "" {
		out.WriteString("  profile: " + profile + "\n")
	}
	if reason != "" {
		out.WriteString("  reason:  " + reason + "\n")
	}
	if profile != "" || reason != "" {
		out.WriteString("\n")
	}
	out.WriteString(indent(raw) + "\n")

	if isLoginProblem {
		out.WriteString("\nLog in first, then start lazyaws again:\n\n")
		out.WriteString("  " + loginCommand(profile) + "\n")
	}

	return out.String()
}

// loginProblem may miss advice because matching is textual, but it never hides the underlying error.
func loginProblem(raw string) (string, bool) {
	lower := strings.ToLower(raw)

	switch {
	case strings.Contains(lower, "failed to refresh cached sso token"),
		strings.Contains(lower, "invalidgrantexception"),
		strings.Contains(lower, "expiredtoken"),
		strings.Contains(lower, "token has expired"):
		return "your SSO session has expired", true
	case strings.Contains(lower, "no credential providers"),
		strings.Contains(lower, "failed to retrieve credentials"),
		strings.Contains(lower, "no aws credentials found"):
		return "no credentials found for this profile", true
	default:
		return "", false
	}
}

func indent(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = "  " + line
	}

	return strings.Join(lines, "\n")
}

func loginCommand(profile string) string {
	if profile == "" {
		return "aws sso login"
	}

	return "aws sso login --profile " + profile
}

func currentProfileName() string {
	return os.Getenv("AWS_PROFILE")
}

func IsStartupFailure(err error) bool {
	var startErr *startupError

	return errors.As(err, &startErr)
}

// clientFailureMessage recommends profile selection when no default exists instead of inventing login advice.
func clientFailureMessage(profile string, cause error, profiles []string) string {
	var out strings.Builder

	out.WriteString("lazyaws can't start.\n\n")
	if profile != "" {
		out.WriteString("  profile: " + profile + "\n\n")
	}
	out.WriteString(indent(strings.TrimSpace(cause.Error())) + "\n")

	switch {
	case profile == "" && len(profiles) > 0:
		out.WriteString("\nNo AWS_PROFILE is set. Pick one of the profiles in ~/.aws/config:\n\n")
		out.WriteString("  AWS_PROFILE=" + profiles[0] + " lazyaws\n\n")
		out.WriteString("  available: " + strings.Join(profiles, ", ") + "\n")
	case len(profiles) == 0:
		out.WriteString("\nNo profiles found in ~/.aws/config. Configure one, then start lazyaws again.\n")
	}

	return out.String()
}
