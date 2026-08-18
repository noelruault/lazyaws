package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/resources"
)

// SecretsActions replaces pending-deletion actions wholesale so unavailable operations cannot be selected.
func (gui *Gui) SecretsActions() []resources.Action {
	secret, err := gui.Panels.Secrets.GetSelectedItem()
	if err != nil {
		return nil
	}

	// Revealing is non-mutating but still asks confirmation because shared terminals expose it to bystanders.
	viewValue := resources.Action{
		Name:         secretsRevealLabel(gui.secretsReveal, secret.Name),
		Confirm:      resources.ConfirmSimple,
		Confirmation: secretsRevealQuestion(gui.secretsReveal, secret.Name),
		Run: func(context.Context, string) error {
			// Action callbacks run off the UI thread, so re-rendering must be queued.
			gui.g.Update(func(*gocui.Gui) error { return gui.toggleSecretReveal() })
			return nil
		},
	}

	if secret.DeletedDate != nil {
		return []resources.Action{viewValue, {
			Name:         "Restore (cancel pending deletion)",
			Mutates:      true,
			Confirm:      resources.ConfirmSimple,
			Confirmation: fmt.Sprintf("Cancel pending deletion for %s?", secret.Name),
			Run:          func(ctx context.Context, _ string) error { return gui.Client.RestoreSecret(ctx, secret.Name) },
		}}
	}

	actions := []resources.Action{
		viewValue,
		{
			Name:         "Rotate secret immediately",
			Mutates:      true,
			Confirm:      resources.ConfirmSimple,
			Confirmation: fmt.Sprintf("Rotate %s immediately?", secret.Name),
			Run:          func(ctx context.Context, _ string) error { return gui.Client.RotateSecret(ctx, secret.Name) },
		},
		{Name: "Edit rotation schedule", Mutates: true, Run: gui.secretsEditRotation(secret)},
		{
			// The recovery window is finite, so deletion requires typing the secret's name.
			Name:    "Delete secret",
			Mutates: true,
			Prompt:  fmt.Sprintf("Recovery window in days (7-30, blank = 30) for %s", secret.Name),
			Confirm: resources.ConfirmDangerous,
			Token:   secret.Name,
			Run: func(ctx context.Context, window string) error {
				days, err := parseSecretsRecoveryWindow(window)
				if err != nil {
					return err
				}
				return gui.Client.DeleteSecret(ctx, secret.Name, days)
			},
		},
		{
			Name:    "Add replica region(s)",
			Mutates: true,
			Prompt:  fmt.Sprintf("Region(s) to add for %s (comma-separated)", secret.Name),
			Run: func(ctx context.Context, input string) error {
				regions, err := parseSecretsRegionList(input)
				if err != nil {
					return err
				}
				return gui.Client.ReplicateSecretToRegions(ctx, secret.Name, regions)
			},
		},
	}

	if secret.HasReplication {
		actions = append(actions, resources.Action{
			Name:    "Remove replica region(s)",
			Mutates: true,
			Prompt:  fmt.Sprintf("Region(s) to remove for %s (comma-separated)", secret.Name),
			Run: func(ctx context.Context, input string) error {
				regions, err := parseSecretsRegionList(input)
				if err != nil {
					return err
				}
				return gui.Client.RemoveSecretReplicaRegions(ctx, secret.Name, regions)
			},
		})
	}

	return actions
}

func secretsRevealLabel(state secretsRevealState, name string) string {
	if state.revealed == name {
		return "Hide value"
	}
	return "View value"
}

func secretsRevealQuestion(state secretsRevealState, name string) string {
	if state.revealed == name {
		return "Mask the value of " + name + " again?"
	}
	return "Show the value of " + name + " in the detail pane?"
}

// toggleSecretReveal relies on the cache key's reveal flag to trigger rendering without direct view mutation.
func (gui *Gui) toggleSecretReveal() error {
	secret, err := gui.Panels.Secrets.GetSelectedItem()
	if err != nil {
		return nil
	}

	gui.secretsReveal = secretsToggleReveal(gui.secretsReveal, secret.Name)

	return gui.Panels.Secrets.HandleSelect()
}

// secretsEditRotation drives two prompts because Action supports only one.
func (gui *Gui) secretsEditRotation(secret *aws.SecretSummary) func(context.Context, string) error {
	return func(_ context.Context, _ string) error {
		gui.g.Update(func(g *gocui.Gui) error {
			return gui.createPromptPanel(fmt.Sprintf("Rotation Lambda ARN for %s", secret.Name), func(g *gocui.Gui, v *gocui.View) error {
				lambdaARN := gui.trimmedContent(v)
				if lambdaARN == "" {
					return gui.createErrorPanel("Lambda ARN is required")
				}

				return gui.runAction(resources.Action{
					Name:         "Set rotation schedule",
					Mutates:      true,
					Prompt:       fmt.Sprintf("Rotate every N days (1-1000) for %s", secret.Name),
					Confirm:      resources.ConfirmSimple,
					Confirmation: fmt.Sprintf("Set %s to rotate automatically using %s?", secret.Name, lambdaARN),
					Run: func(ctx context.Context, input string) error {
						days, err := parseSecretsRotationDays(input)
						if err != nil {
							return err
						}
						return gui.Client.ConfigureSecretRotation(ctx, secret.Name, lambdaARN, days)
					},
				})
			})
		})

		return nil
	}
}

// parseSecretsRecoveryWindow maps blank to AWS's 30-day default and enforces its 7-30 range.
func parseSecretsRecoveryWindow(input string) (int64, error) {
	if input == "" {
		return 0, nil
	}
	days, err := strconv.ParseInt(input, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number of days: %q", input)
	}
	if days < 7 || days > 30 {
		return 0, fmt.Errorf("recovery window must be 7-30 days, got %d", days)
	}
	return days, nil
}

// parseSecretsRotationDays enforces AWS's 1-1000 day range.
func parseSecretsRotationDays(input string) (int32, error) {
	days, err := strconv.ParseInt(input, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid number of days: %q", input)
	}
	if days < 1 || days > 1000 {
		return 0, fmt.Errorf("rotation interval must be 1-1000 days, got %d", days)
	}
	return int32(days), nil
}

func parseSecretsRegionList(input string) ([]string, error) {
	var regions []string
	for _, part := range strings.Split(input, ",") {
		r := strings.TrimSpace(part)
		if r != "" {
			regions = append(regions, r)
		}
	}
	if len(regions) == 0 {
		return nil, fmt.Errorf("at least one region code is required")
	}
	return regions, nil
}

func (gui *Gui) handleSecretsToggleReveal(g *gocui.Gui, v *gocui.View) error {
	return gui.toggleSecretReveal()
}
