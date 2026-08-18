// Package q streams non-interactive answers from the Kiro CLI; disabling CLI wrapping preserves the TUI's line-to-click mapping.
package q

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	Binary = "kiro-cli"

	DefaultTimeout = 5 * time.Minute

	// gracePeriod bounds cancellation cleanup so a hung child cannot block Stream.
	gracePeriod = 2 * time.Second

	// maxLineBytes exceeds Scanner's default because --wrap never can emit whole paragraphs.
	maxLineBytes = 1 << 20
)

var ErrNotInstalled = errors.New("kiro-cli not found on PATH: install it from https://kiro.dev/docs/cli/ and sign in with `kiro-cli login`")

// ansiEscape matches the ANSI escape sequences q writes for colour and cursor movement; the TUI renders the text itself, so they are stripped.
var ansiEscape = regexp.MustCompile(`\x1B(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~])`)

type Request struct {
	// Prompt must be non-empty so invalid requests fail before spawning a child.
	Prompt  string
	Context string
	// Profile and Region preserve the inherited AWS environment when empty.
	Profile string
	Region  string
}

func Available() bool {
	_, err := exec.LookPath(Binary)
	return err == nil
}

// Stream kills the child on cancellation and sends callbacks ANSI-free complete lines.
func Stream(ctx context.Context, req Request, onLine func(string)) error {
	if strings.TrimSpace(req.Prompt) == "" {
		return errors.New("empty prompt")
	}
	if !Available() {
		return ErrNotInstalled
	}

	prompt := req.Prompt
	if req.Context != "" {
		prompt = req.Context + "\n\n" + req.Prompt
	}

	cmd := exec.CommandContext(ctx, Binary, "chat", "--no-interactive", "--trust-all-tools", "--wrap", "never", prompt)
	cmd.Env = childEnv(req.Profile, req.Region)
	// A cancelled chat has nothing to flush; WaitDelay only bounds child tools that keep the pipe open.
	cmd.WaitDelay = gracePeriod

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxLineBytes)
	for scanner.Scan() {
		if onLine != nil {
			onLine(StripANSI(scanner.Text()))
		}
	}
	scanErr := scanner.Err()

	if err := cmd.Wait(); err != nil {
		// A cancelled or timed-out query surfaces as the context's error rather than q's uninformative "signal: killed" exit status.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if msg := strings.TrimSpace(StripANSI(stderr.String())); msg != "" {
			return fmt.Errorf("%s: %s", Binary, msg)
		}
		return fmt.Errorf("%s: %w", Binary, err)
	}

	return scanErr
}

// FormatContext returns "" when no AWS identity is known.
func FormatContext(profile, region, account string) string {
	var b strings.Builder
	add := func(label, value string) {
		if value == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(label)
		b.WriteByte(' ')
		b.WriteString(value)
	}

	add("profile", profile)
	add("region", region)
	add("account", account)

	if b.Len() == 0 {
		return ""
	}

	return "You are answering about a live AWS environment — " + b.String() + "."
}

func StripANSI(text string) string {
	return ansiEscape.ReplaceAllString(text, "")
}

// childEnv appends identity because os/exec keeps the last duplicate environment key.
func childEnv(profile, region string) []string {
	env := os.Environ()
	if profile != "" {
		env = append(env, "AWS_PROFILE="+profile)
	}
	if region != "" {
		env = append(env, "AWS_DEFAULT_REGION="+region)
	}
	return env
}
