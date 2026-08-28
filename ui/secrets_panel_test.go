package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/aws/smithy-go"

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

// GetSecretDetails never fetches the value, so the policy render must not leak a value-shaped field.
func TestFormatSecretPolicyNeverIncludesValue(t *testing.T) {
	out := formatSecretPolicy(&awsapp.SecretDetails{ResourcePolicy: `{"Version":"2012-10-17"}`})

	if strings.Contains(strings.ToLower(out), "secretstring") {
		t.Errorf("formatSecretPolicy leaked a value-shaped field:\n%s", out)
	}
	if !strings.Contains(out, "2012-10-17") {
		t.Errorf("expected the resource policy in the output, got:\n%s", out)
	}
}

// The Policy tab publishes the same field as the Overview, so it renders the same three states: a failed read left saying "not configured" here would contradict the pane next to it.
func TestFormatSecretPolicySeparatesAnUnreadablePolicyFromAnAbsentOne(t *testing.T) {
	failed := formatSecretPolicy(&awsapp.SecretDetails{ResourcePolicyErr: errors.New("AccessDenied")})
	absent := formatSecretPolicy(&awsapp.SecretDetails{})

	if !strings.Contains(failed, "unavailable: AccessDenied") {
		t.Errorf("a policy read that failed does not say so:\n%s", failed)
	}
	if strings.Contains(failed, "not configured") {
		t.Errorf("a policy read that failed still renders as an absence:\n%s", failed)
	}
	if !strings.Contains(absent, "not configured") {
		t.Errorf("a secret with no policy no longer states the absence:\n%s", absent)
	}
}

// A throttled GetResourcePolicy never fails the overview, so nothing else would carry it to the gate that paces the pane's next fetch.
func TestSecretOverviewReportsAThrottledPolicyReadToTheBackoffEngine(t *testing.T) {
	var watch throttleWatch
	details := &awsapp.SecretDetails{ResourcePolicyErr: &smithy.GenericAPIError{Code: "ThrottlingException"}}

	watch.observe(secretOverviewErrs(details, nil)...)

	throttled, reported := watch.take()
	if !reported || !throttled {
		t.Errorf("a throttled policy read did not reach the backoff engine: throttled=%v reported=%v", throttled, reported)
	}
}

func TestSecretOverviewErrs(t *testing.T) {
	fetchErr, policyErr := errors.New("describe failed"), errors.New("policy read failed")

	for _, tt := range []struct {
		name    string
		details *awsapp.SecretDetails
		err     error
		want    error
	}{
		{name: "the fetch failed", details: nil, err: fetchErr, want: fetchErr},
		{name: "only the policy read failed", details: &awsapp.SecretDetails{ResourcePolicyErr: policyErr}, want: policyErr},
		{name: "nothing failed", details: &awsapp.SecretDetails{}, want: nil},
	} {
		got := secretOverviewErrs(tt.details, tt.err)
		if len(got) != 1 || got[0] != tt.want {
			t.Errorf("secretOverviewErrs(%s) = %v, want [%v]", tt.name, got, tt.want)
		}
	}
}
