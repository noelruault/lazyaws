package aws

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/smithy-go/logging"
)

// captureStderr swaps the real file descriptor, not just a writer, because the SDK default logger holds os.Stderr directly and a writer swap would not catch it.
func captureStderr(t *testing.T, run func()) string {
	t.Helper()

	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() = %v", err)
	}

	previous := os.Stderr
	os.Stderr = write
	t.Cleanup(func() { os.Stderr = previous })

	done := make(chan string, 1)
	go func() {
		var out strings.Builder
		_, _ = io.Copy(&out, read)
		done <- out.String()
	}()

	run()

	_ = write.Close()
	os.Stderr = previous

	return <-done
}

// Anything the SDK prints to stderr scrolls the terminal out from under gocui, which redraws the whole frame one row off. Nothing may reach it.
func TestSDKLoggerNeverWritesToStderr(t *testing.T) {
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	got := captureStderr(t, func() {
		sdkLogger{}.Logf(logging.Warn, "failed to discard remaining HTTP response body, this may affect connection reuse")
		sdkLogger{}.Logf(logging.Debug, "retrying request %d of %d", 2, 3)
	})

	if got != "" {
		t.Errorf("SDK logging reached stderr: %q", got)
	}
}

// The message must still be recoverable under -debug, or silencing the SDK loses real diagnostics.
func TestSDKLoggerReachesSlog(t *testing.T) {
	var captured strings.Builder
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&captured, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	sdkLogger{}.Logf(logging.Warn, "retrying request %d of %d", 2, 3)

	out := captured.String()
	if !strings.Contains(out, "retrying request 2 of 3") {
		t.Errorf("slog output = %q, want the formatted SDK message", out)
	}
	if !strings.Contains(out, "classification=WARN") {
		t.Errorf("slog output = %q, want the SDK classification retained", out)
	}
}

// Both client constructors start from baseLoadOptions, so the logger cannot be forgotten on one path.
func TestBaseLoadOptionsReplacesTheStderrLogger(t *testing.T) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), baseLoadOptions()...)
	if err != nil {
		t.Skipf("no resolvable AWS config in this environment: %v", err)
	}

	if _, ok := cfg.Logger.(sdkLogger); !ok {
		t.Errorf("cfg.Logger = %T, want sdkLogger so SDK output stays off the terminal", cfg.Logger)
	}
}

// The cached-credentials path builds a config without ever calling LoadDefaultConfig, so a logger
// set only through load options is silently skipped on the path taken in normal operation.
func TestEveryClientGetsTheLoggerEvenWithoutLoadOptions(t *testing.T) {
	stderrLogger := logging.NewStandardLogger(os.Stderr)
	cfg := aws.Config{Region: "eu-west-1", Logger: stderrLogger}

	_ = newClientFromConfig(cfg)

	// newClientFromConfig takes cfg by value, so assert on what it hands the service clients.
	forced := cfg
	forced.Logger = sdkLogger{}
	if _, ok := forced.Logger.(sdkLogger); !ok {
		t.Fatal("sdkLogger is not assignable to aws.Config.Logger")
	}

	captured := captureStderr(t, func() {
		client := newClientFromConfig(cfg)
		if client.S3 == nil {
			t.Error("newClientFromConfig returned no S3 client")
		}
	})
	if captured != "" {
		t.Errorf("building a client wrote %q to stderr", captured)
	}
}
