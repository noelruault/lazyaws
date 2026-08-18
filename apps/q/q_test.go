package q

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Fake scripts expose real argv and environment without requiring the CLI.
func fakeQ(t *testing.T, body string) string {
	t.Helper()

	return fakeCLI(t, body, Binary)
}

func fakeCLI(t *testing.T, body string, names ...string) string {
	t.Helper()

	dir := t.TempDir()
	for _, name := range names {
		script := "#!/bin/sh\n" + body + "\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatalf("writing fake %s: %v", name, err)
		}
	}
	t.Setenv("PATH", dir)

	return dir
}

// The retired q binary must not shadow kiro-cli discovery.
func TestAvailable(t *testing.T) {
	t.Run("installed", func(t *testing.T) {
		fakeCLI(t, "exit 0", Binary)

		if !Available() {
			t.Errorf("Available() = false with %s on PATH", Binary)
		}
	})

	t.Run("only the retired q", func(t *testing.T) {
		fakeCLI(t, "exit 0", "q")

		if Available() {
			t.Error("Available() = true with only the retired q on PATH, want the current CLI required")
		}
	})

	t.Run("not installed", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())

		if Available() {
			t.Error("Available() = true with an empty PATH")
		}
	})
}

func TestStreamNamesTheCLIThatFailed(t *testing.T) {
	fakeQ(t, `printf 'boom\n' >&2; exit 1`)

	err := Stream(context.Background(), Request{Prompt: "hi"}, nil)
	if err == nil {
		t.Fatal("Stream() error = nil, want the CLI's failure")
	}
	if !strings.HasPrefix(err.Error(), Binary+": ") {
		t.Errorf("error = %q, want it to name %q", err, Binary)
	}
}

func TestStreamCollectsLines(t *testing.T) {
	fakeQ(t, `printf 'first line\n\033[32msecond line\033[0m\nthird line\n'`)

	var got []string
	err := Stream(context.Background(), Request{Prompt: "how do I list buckets?"}, func(line string) {
		got = append(got, line)
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	want := []string{"first line", "second line", "third line"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("lines = %q, want %q", got, want)
	}
}

// Non-interactive flags prevent an unattended child from waiting for terminal input.
func TestStreamPassesDocumentedFlagsAndPrompt(t *testing.T) {
	fakeQ(t, `for arg in "$@"; do echo "$arg"; done`)

	var got []string
	err := Stream(context.Background(), Request{
		Prompt:  "which instances are running?",
		Context: "Context: Current AWS Profile: prod, Region: eu-west-1",
	}, func(line string) {
		got = append(got, line)
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	want := []string{
		"chat",
		"--no-interactive",
		"--trust-all-tools",
		"--wrap",
		"never",
		"Context: Current AWS Profile: prod, Region: eu-west-1",
		"",
		"which instances are running?",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("argv+prompt =\n%q\nwant\n%q", got, want)
	}
}

// Request identity must override the inherited environment.
func TestStreamOverridesProfileAndRegion(t *testing.T) {
	fakeQ(t, `echo "$AWS_PROFILE $AWS_DEFAULT_REGION"`)
	t.Setenv("AWS_PROFILE", "inherited")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")

	var got string
	err := Stream(context.Background(), Request{Prompt: "hi", Profile: "prod", Region: "eu-west-1"}, func(line string) {
		got = line
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	if got != "prod eu-west-1" {
		t.Errorf("child env = %q, want %q", got, "prod eu-west-1")
	}
}

func TestStreamInheritsProfileWhenUnset(t *testing.T) {
	fakeQ(t, `echo "$AWS_PROFILE"`)
	t.Setenv("AWS_PROFILE", "inherited")

	var got string
	err := Stream(context.Background(), Request{Prompt: "hi"}, func(line string) { got = line })
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	if got != "inherited" {
		t.Errorf("AWS_PROFILE = %q, want %q", got, "inherited")
	}
}

func TestStreamReportsStderrOnFailure(t *testing.T) {
	fakeQ(t, `printf 'partial answer\n'; printf '\033[31mnot authenticated, run q login\033[0m\n' >&2; exit 1`)

	var got []string
	err := Stream(context.Background(), Request{Prompt: "hi"}, func(line string) { got = append(got, line) })
	if err == nil {
		t.Fatal("Stream() error = nil, want the CLI's failure")
	}
	if !strings.Contains(err.Error(), "not authenticated, run q login") {
		t.Errorf("error = %q, want it to carry q's stderr", err)
	}
	if strings.Contains(err.Error(), "\x1b") {
		t.Errorf("error = %q, want ANSI codes stripped", err)
	}
	// Preserve useful output emitted before the child fails.
	if len(got) != 1 || got[0] != "partial answer" {
		t.Errorf("lines = %q, want the pre-failure output", got)
	}
}

// Cancellation must report the context reason instead of the child's kill signal.
func TestStreamCancellation(t *testing.T) {
	fakeQ(t, `echo "thinking"; sleep 30`)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Stream(ctx, Request{Prompt: "hi"}, func(line string) { cancel() })
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Stream() error = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Stream() did not return after cancellation")
	}
}

func TestStreamRejectsEmptyPrompt(t *testing.T) {
	fakeQ(t, `echo "should not run"`)

	called := false
	err := Stream(context.Background(), Request{Prompt: "   "}, func(string) { called = true })
	if err == nil {
		t.Error("Stream() error = nil, want a rejection of the empty prompt")
	}
	if called {
		t.Error("Stream() ran q for an empty prompt")
	}
}

func TestStreamWithoutCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if err := Stream(context.Background(), Request{Prompt: "hi"}, nil); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("Stream() error = %v, want ErrNotInstalled", err)
	}
}

func TestFormatContext(t *testing.T) {
	tests := []struct {
		name                     string
		profile, region, account string
		want                     string
	}{
		{"all", "prod", "eu-west-1", "111111111111", "You are answering about a live AWS environment — profile prod | region eu-west-1 | account 111111111111."},
		{"no account", "prod", "eu-west-1", "", "You are answering about a live AWS environment — profile prod | region eu-west-1."},
		{"region only", "", "eu-west-1", "", "You are answering about a live AWS environment — region eu-west-1."},
		{"nothing known", "", "", "", ""},
	}

	for _, tt := range tests {
		if got := FormatContext(tt.profile, tt.region, tt.account); got != tt.want {
			t.Errorf("%s: FormatContext() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"\x1b[32mgreen\x1b[0m", "green"},
		{"\x1b[1;31mbold red\x1b[0m text", "bold red text"},
		{"\x1b[2K\x1b[1Gspinner", "spinner"},
		{"plain", "plain"},
	}

	for _, tt := range tests {
		if got := StripANSI(tt.in); got != tt.want {
			t.Errorf("StripANSI(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
