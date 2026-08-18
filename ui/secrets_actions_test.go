package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/config"
	"github.com/noelruault/lazyaws/ui/resources"
	"github.com/noelruault/lazyaws/ui/types"
)

func TestParseSecretsRecoveryWindow(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{name: "blank uses AWS default", input: "", want: 0},
		{name: "minimum", input: "7", want: 7},
		{name: "maximum", input: "30", want: 30},
		{name: "too low", input: "6", wantErr: true},
		{name: "too high", input: "31", wantErr: true},
		{name: "not a number", input: "soon", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSecretsRecoveryWindow(tt.input)
			if tt.wantErr != (err != nil) {
				t.Fatalf("parseSecretsRecoveryWindow(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseSecretsRecoveryWindow(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseSecretsRotationDays(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int32
		wantErr bool
	}{
		{name: "minimum", input: "1", want: 1},
		{name: "maximum", input: "1000", want: 1000},
		{name: "too low", input: "0", wantErr: true},
		{name: "too high", input: "1001", wantErr: true},
		{name: "blank", input: "", wantErr: true},
		{name: "not a number", input: "often", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSecretsRotationDays(tt.input)
			if tt.wantErr != (err != nil) {
				t.Fatalf("parseSecretsRotationDays(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseSecretsRotationDays(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseSecretsRegionList(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{name: "single", input: "us-west-2", want: []string{"us-west-2"}},
		{name: "comma-separated with spaces", input: "us-west-2, eu-west-1 ,ap-southeast-1", want: []string{"us-west-2", "eu-west-1", "ap-southeast-1"}},
		{name: "blank", input: "", wantErr: true},
		{name: "only commas", input: " , ,", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSecretsRegionList(tt.input)
			if tt.wantErr != (err != nil) {
				t.Fatalf("parseSecretsRegionList(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("parseSecretsRegionList(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSecretsDeletePrompt(t *testing.T) {

}

// Viewing a value must survive read-only filtering because it does not mutate AWS.
func TestSecretsViewValueSurvivesReadOnly(t *testing.T) {
	user := config.DefaultUserConfig()
	user.ReadOnly = true
	gui, g := newHeadlessGuiWithConfig(t, user)

	run(t, g, func() error {
		gui.Panels.Secrets.SetItems([]*aws.SecretSummary{{Name: "db-password"}})
		return nil
	})

	actions := ask(g, gui.SecretsActions)
	view, ok := findAction(actions, "View value")
	if !ok {
		t.Fatalf("secrets actions = %v, want View value among them", actionNames(actions))
	}
	if view.Mutates {
		t.Error("View value is marked mutating, so read-only mode would hide the one thing it should still allow")
	}
	// Shared terminals require confirmation before revealing plaintext.
	if view.Confirm != resources.ConfirmSimple {
		t.Errorf("View value confirm = %v, want simple", view.Confirm)
	}

	kept := ask(g, func() []*types.MenuItem { return gui.dropMutatingItems(gui.actionMenuItems(actions)) })
	var labels []string
	for _, item := range kept {
		labels = append(labels, item.Label)
	}
	if !hasLabelContaining(labels, "View value") {
		t.Errorf("read-only kept %v, want View value among them", labels)
	}
}

func TestSecretsViewValueSaysWhichWayItGoes(t *testing.T) {
	masked := secretsRevealState{}
	if got := secretsRevealLabel(masked, "db-password"); got != "View value" {
		t.Errorf("label while masked = %q", got)
	}

	revealed := secretsRevealState{revealed: "db-password"}
	if got := secretsRevealLabel(revealed, "db-password"); got != "Hide value" {
		t.Errorf("label while revealed = %q", got)
	}
	if got := secretsRevealLabel(revealed, "other"); got != "View value" {
		t.Errorf("label for a different secret = %q", got)
	}
}

func TestSecretsViewValueIsOfferedOnADeletedSecret(t *testing.T) {
	gui, g := newHeadlessGui(t)

	deleted := time.Now()
	run(t, g, func() error {
		gui.Panels.Secrets.SetItems([]*aws.SecretSummary{{Name: "db-password", DeletedDate: &deleted}})
		return nil
	})

	actions := ask(g, gui.SecretsActions)
	if _, ok := findAction(actions, "View value"); !ok {
		t.Errorf("actions on a secret pending deletion = %v, want View value among them", actionNames(actions))
	}
}
