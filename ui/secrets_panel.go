// Package ui keeps secret metadata rendering plaintext-free until explicit reveal.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/presentation"
	"github.com/noelruault/lazyaws/ui/tasks"
	"github.com/noelruault/lazyaws/ui/utils"
)

type secretsRevealState struct {
	lastItem string
	revealed string
}

// secretsHandleSelect re-masks on selection changes but preserves same-secret tab state.
func secretsHandleSelect(state secretsRevealState, name string) secretsRevealState {
	if state.lastItem != name {
		return secretsRevealState{lastItem: name}
	}
	return state
}

func secretsToggleReveal(state secretsRevealState, name string) secretsRevealState {
	state.lastItem = name
	if state.revealed == name {
		state.revealed = ""
	} else {
		state.revealed = name
	}
	return state
}

func (gui *Gui) getSecretsPanel() *panels.SideListPanel[*aws.SecretSummary] {
	return &panels.SideListPanel[*aws.SecretSummary]{
		ContextState: &panels.ContextState[*aws.SecretSummary]{
			GetMainTabs: func() []panels.MainTab[*aws.SecretSummary] {
				return []panels.MainTab[*aws.SecretSummary]{
					overviewTab(gui, func(s *aws.SecretSummary) string { return "secret-" + s.Name }, gui.secretOverview),
					{Key: "value", Title: "Value", Render: gui.renderSecretValue},
					{Key: "versions", Title: "Versions", Render: gui.renderSecretVersions},
					{Key: "policy", Title: "Policy", Render: gui.renderSecretPolicy},
				}
			},
			GetItemContextCacheKey: func(s *aws.SecretSummary) string {
				revealed := gui.secretsReveal.revealed == s.Name
				return fmt.Sprintf("secrets-%s-%v", s.Name, revealed)
			},
		},

		ListPanel: panels.ListPanel[*aws.SecretSummary]{
			List: panels.NewFilteredList[*aws.SecretSummary](),
			View: gui.Views.Secrets,
		},
		NoItemsMessage: "no secrets",
		Gui:            gui.intoInterface(),

		OnSelect: func(s *aws.SecretSummary) error {
			gui.secretsReveal = secretsHandleSelect(gui.secretsReveal, s.Name)
			return nil
		},

		Sort: func(a, b *aws.SecretSummary) bool {
			return a.Name < b.Name
		},
		GetTableCellsFit: func(s *aws.SecretSummary) []utils.Cell {
			return presentation.GetSecretDisplayCells(s)
		},
		Weights:   func(*aws.SecretSummary) []int { return presentation.SecretWeights() },
		CopyValue: func(s *aws.SecretSummary) string { return arnOrName(s.Arn, s.Name) },
	}
}

func (gui *Gui) loadSecretsList() error {
	if gui.Client == nil {
		return nil
	}

	gen := gui.Gen

	return gui.WithWaitingStatus("loading secrets", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		secrets, err := gui.Client.ListSecrets(ctx, gui.secretsShowDeleted)
		if err != nil {
			return err
		}
		if gen != gui.Gen {
			return nil
		}

		rows := make([]*aws.SecretSummary, len(secrets))
		for i := range secrets {
			rows[i] = &secrets[i]
		}
		gui.Panels.Secrets.SetItemsKeepSelection(rows, secretsSelectionKey)
		return gui.Panels.Secrets.RerenderList()
	})
}

// secretsSelectionKey identifies a secret across reloads by name rather than ARN, because the same name reappears with a new ARN after a delete and recreate.
func secretsSelectionKey(secret *aws.SecretSummary) string { return secret.Name }

func (gui *Gui) handleSecretsToggleDeleted(g *gocui.Gui, v *gocui.View) error {
	gui.secretsShowDeleted = !gui.secretsShowDeleted
	return gui.loadSecretsList()
}

// secretOverview reads the same metadata the Config tab does and never the value, so an overview that re-renders on its own interval still emits no GetSecretValue CloudTrail event.
func (gui *Gui) secretOverview(ctx context.Context, secret *aws.SecretSummary, width int) string {
	if gui.Client == nil {
		return overviewUnavailable("secret")
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	details, err := gui.Client.GetSecretDetails(fetchCtx, secret.Name)
	gui.throttles.observe(secretOverviewErrs(details, err)...)
	if err != nil {
		return overviewUnavailableBecause("secret", err)
	}

	return presentation.FormatSecretOverview(details, width, time.Now())
}

// secretOverviewErrs is everything one overview fetch can be throttled on, which is not the same as everything that can fail it: the best-effort resource-policy read is dropped by GetSecretDetails' own error, so a throttle on it would otherwise never reach the backoff engine and the pane would keep asking at full rate.
func secretOverviewErrs(details *aws.SecretDetails, err error) []error {
	if err != nil || details == nil {
		return []error{err}
	}

	return []error{details.ResourcePolicyErr}
}

// renderSecretsConfig avoids GetSecretValue so browsing emits no value-read CloudTrail event.
// renderSecretVersions shows the rotation history whole; the Overview caps it at a glance's worth.
func (gui *Gui) renderSecretVersions(secret *aws.SecretSummary) tasks.TaskFunc {
	name := secret.Name
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		details, err := gui.Client.GetSecretDetails(fetchCtx, name)
		if gen != gui.Gen {
			return
		}
		if err != nil {
			gui.RenderStringMain("error loading secret: " + err.Error())
			return
		}
		gui.RenderStringMain(presentation.FormatSecretVersions(details, gui.Views.Main.InnerWidth(), time.Now()))
	}})
}

// renderSecretPolicy shows the resource policy whole, which is the one thing the Overview only reports the presence of.
func (gui *Gui) renderSecretPolicy(secret *aws.SecretSummary) tasks.TaskFunc {
	name := secret.Name
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		details, err := gui.Client.GetSecretDetails(fetchCtx, name)
		if gen != gui.Gen {
			return
		}
		if err != nil {
			gui.RenderStringMain("error loading secret: " + err.Error())
			return
		}
		gui.RenderStringMain(formatSecretPolicy(details))
	}})
}

func formatSecretsTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format(time.RFC3339)
}

func formatSecretPolicy(d *aws.SecretDetails) string {
	switch {
	case d.ResourcePolicyErr != nil:
		return "unavailable: " + d.ResourcePolicyErr.Error() + "\n"
	case d.ResourcePolicy == "":
		return "not configured\n"
	default:
		return d.ResourcePolicy + "\n"
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// renderSecretValue confines plaintext to the task closure instead of panel state.
func (gui *Gui) renderSecretValue(secret *aws.SecretSummary) tasks.TaskFunc {
	if gui.secretsReveal.revealed != secret.Name {
		return gui.NewSimpleRenderStringTask(func() string {
			return "value hidden — press v to reveal\n"
		})
	}

	name := secret.Name
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		value, prettyJSON, err := gui.Client.GetSecretValueString(fetchCtx, name)
		if gen != gui.Gen {
			return
		}
		if err != nil {
			gui.RenderStringMain("error loading value: " + err.Error())
			return
		}
		gui.RenderStringMain(formatSecretValue(value, prettyJSON))
	}})
}

func formatSecretValue(value, prettyJSON string) string {
	body := value + "\n"
	if prettyJSON != "" {
		body = prettyJSON + "\n"
	} else if value == "" {
		body = "(empty)\n"
	}

	return secretFidelityWarning(value) + body
}

// secretFidelityWarning covers what cleanString strips downstream, because a silently shortened credential looks exactly like a correct one.
func secretFidelityWarning(value string) string {
	const byteOrderMark = rune(0xFEFF)

	var dropped []string
	if strings.ContainsRune(value, '\r') {
		dropped = append(dropped, "carriage returns")
	}
	if strings.HasPrefix(value, string(byteOrderMark)) {
		dropped = append(dropped, "a leading byte-order mark")
	}
	if len(dropped) == 0 {
		return ""
	}

	warning := "this value contains " + strings.Join(dropped, " and ") + ", which the pane cannot show; read it with the AWS CLI instead of copying from here"
	return utils.ColoredString(warning, color.FgYellow) + "\n\n"
}
