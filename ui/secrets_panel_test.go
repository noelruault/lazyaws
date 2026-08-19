package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	secretsmanagertypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"

	awsapp "github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// Wrong transitions here mean a secret's value stays visible after navigating away, or `v` fails to reveal or re-mask.
func TestSecretsHandleSelect(t *testing.T) {
	tests := []struct {
		name  string
		start secretsRevealState
		item  string
		want  secretsRevealState
	}{
		{
			name:  "different item re-masks",
			start: secretsRevealState{lastItem: "a", revealed: "a"},
			item:  "b",
			want:  secretsRevealState{lastItem: "b"},
		},
		{
			name:  "same item (tab switch) leaves reveal untouched",
			start: secretsRevealState{lastItem: "a", revealed: "a"},
			item:  "a",
			want:  secretsRevealState{lastItem: "a", revealed: "a"},
		},
		{
			name:  "first selection",
			start: secretsRevealState{},
			item:  "a",
			want:  secretsRevealState{lastItem: "a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := secretsHandleSelect(tt.start, tt.item); got != tt.want {
				t.Errorf("secretsHandleSelect(%+v, %q) = %+v, want %+v", tt.start, tt.item, got, tt.want)
			}
		})
	}
}

func TestSecretsToggleReveal(t *testing.T) {
	tests := []struct {
		name  string
		start secretsRevealState
		item  string
		want  secretsRevealState
	}{
		{
			name:  "reveal masked",
			start: secretsRevealState{lastItem: "a"},
			item:  "a",
			want:  secretsRevealState{lastItem: "a", revealed: "a"},
		},
		{
			name:  "re-mask revealed",
			start: secretsRevealState{lastItem: "a", revealed: "a"},
			item:  "a",
			want:  secretsRevealState{lastItem: "a"},
		},
		{
			name:  "toggle on a different item reveals that one",
			start: secretsRevealState{lastItem: "a", revealed: "a"},
			item:  "b",
			want:  secretsRevealState{lastItem: "b", revealed: "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := secretsToggleReveal(tt.start, tt.item); got != tt.want {
				t.Errorf("secretsToggleReveal(%+v, %q) = %+v, want %+v", tt.start, tt.item, got, tt.want)
			}
		})
	}
}

func TestFormatSecretValue(t *testing.T) {
	if got := formatSecretValue("plain", ""); got != "plain\n" {
		t.Errorf("formatSecretValue(plain) = %q", got)
	}
	if got := formatSecretValue(`{"a":1}`, "{\n  \"a\": 1\n}"); got != "{\n  \"a\": 1\n}\n" {
		t.Errorf("formatSecretValue(json) = %q", got)
	}
	if got := formatSecretValue("", ""); got != "(empty)\n" {
		t.Errorf("formatSecretValue(empty) = %q", got)
	}
}

// gocui reads a carriage return as a cursor move, so the render pipeline strips it downstream.
// The value on screen is then shorter than the value stored, and nothing says so: a truncated credential looks exactly like a correct one, which is the same failure as re-encoding it.
func TestFormatSecretValueWarnsWhenTheTerminalCannotShowItFaithfully(t *testing.T) {
	pem := "-----BEGIN CERTIFICATE-----\r\nMIIBkTCB+w==\r\n-----END CERTIFICATE-----\r\n"

	got := utils.Decolorise(formatSecretValue(pem, ""))

	if !strings.Contains(got, "carriage returns") {
		t.Errorf("a value the pane cannot reproduce must say so:\n%s", got)
	}
	// The value itself still shows; the warning is additional, not a replacement.
	if !strings.Contains(got, "BEGIN CERTIFICATE") {
		t.Errorf("the value must still render:\n%s", got)
	}
}

func TestFormatSecretValueWarnsAboutALeadingBOM(t *testing.T) {
	// Built from the code point because a literal byte-order mark in Go source is a compile error.
	value := string([]rune{0xFEFF}) + "hunter2"

	got := utils.Decolorise(formatSecretValue(value, ""))

	if !strings.Contains(got, "byte-order mark") {
		t.Errorf("a stripped BOM must be reported:\n%s", got)
	}
}

// A JSON secret escapes its control characters at rest, so those survive the pane untouched and must not draw a warning that would train the reader to ignore it.
func TestFormatSecretValueStaysQuietWhenNothingIsLost(t *testing.T) {
	for _, value := range []string{"plain", `{"password":"a\rb"}`, ""} {
		got := utils.Decolorise(formatSecretValue(value, ""))
		if strings.Contains(got, "cannot show") {
			t.Errorf("formatSecretValue(%q) warned needlessly:\n%s", value, got)
		}
	}
}

func TestFormatSecretRotation(t *testing.T) {
	if got := formatSecretRotation(&awsapp.SecretDetails{}); got != "disabled" {
		t.Errorf("formatSecretRotation(disabled) = %q", got)
	}

	enabled := &awsapp.SecretDetails{SecretSummary: awsapp.SecretSummary{RotationEnabled: true}}
	if got := formatSecretRotation(enabled); got != "enabled" {
		t.Errorf("formatSecretRotation(enabled, no rules) = %q", got)
	}

	withDays := &awsapp.SecretDetails{
		SecretSummary: awsapp.SecretSummary{RotationEnabled: true},
		Rotation:      &secretsmanagertypes.RotationRulesType{AutomaticallyAfterDays: aws.Int64(30)},
	}
	if got := formatSecretRotation(withDays); got != "enabled, every 30 day(s)" {
		t.Errorf("formatSecretRotation(30 days) = %q", got)
	}
}

func TestFormatSecretsConfigNeverIncludesValue(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	details := &awsapp.SecretDetails{
		SecretSummary: awsapp.SecretSummary{
			Name:      "db-password",
			Arn:       "arn:aws:secretsmanager:us-east-1:1:secret:db-password",
			CreatedAt: &created,
			KMSKeyID:  "alias/app",
		},
		Versions: []secretsmanagertypes.SecretVersionsListEntry{
			{VersionId: aws.String("v1"), VersionStages: []string{"AWSCURRENT"}, CreatedDate: &created},
		},
		ResourcePolicy: `{"Version":"2012-10-17"}`,
	}

	out := formatSecretsConfig(details)

	// GetSecretDetails never fetches the value, so the config render must not leak a value-shaped field.
	if strings.Contains(strings.ToLower(out), "secretstring") {
		t.Errorf("formatSecretsConfig leaked a value-shaped field:\n%s", out)
	}
	if !strings.Contains(out, "db-password") || !strings.Contains(out, "AWSCURRENT") || !strings.Contains(out, "2012-10-17") {
		t.Errorf("expected name, version stage, and resource policy in output, got:\n%s", out)
	}
}

func TestFormatSecretsConfigDeletedStatus(t *testing.T) {
	deleted := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	details := &awsapp.SecretDetails{
		SecretSummary: awsapp.SecretSummary{Name: "old-key", DeletedDate: &deleted},
	}
	out := formatSecretsConfig(details)
	if !strings.Contains(out, "pending deletion") {
		t.Errorf("expected pending-deletion status, got:\n%s", out)
	}
}
