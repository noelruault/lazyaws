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
					{Key: "config", Title: "Config", Render: gui.renderSecretsConfig},
					{Key: "value", Title: "Value", Render: gui.renderSecretValue},
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
		GetTableCells: func(s *aws.SecretSummary) []string {
			return presentation.GetSecretDisplayStrings(s)
		},
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
		gui.Panels.Secrets.SetItems(rows)
		return gui.Panels.Secrets.RerenderList()
	})
}

func (gui *Gui) handleSecretsToggleDeleted(g *gocui.Gui, v *gocui.View) error {
	gui.secretsShowDeleted = !gui.secretsShowDeleted
	return gui.loadSecretsList()
}

// renderSecretsConfig avoids GetSecretValue so browsing emits no value-read CloudTrail event.
func (gui *Gui) renderSecretsConfig(secret *aws.SecretSummary) tasks.TaskFunc {
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
		gui.RenderStringMain(formatSecretsConfig(details))
	}})
}

func formatSecretsConfig(d *aws.SecretDetails) string {
	consoleURL := ""
	if d.PrimaryRegion != "" {
		consoleURL = fmt.Sprintf("https://%s.console.aws.amazon.com/secretsmanager/secret?name=%s&region=%s", d.PrimaryRegion, d.Name, d.PrimaryRegion)
	}

	fields := map[string]string{
		"Name":         d.Name,
		"ARN":          d.Arn,
		"Description":  orDash(d.Description),
		"Created":      formatSecretsTime(d.CreatedAt),
		"Last changed": formatSecretsTime(d.LastChanged),
		"KMS key":      orDash(d.KMSKeyID),
		"Rotation":     formatSecretRotation(d),
		"Last rotated": formatSecretsTime(d.LastRotated),
		"Console":      consoleURL,
	}
	if d.DeletedDate != nil {
		fields["Status"] = "pending deletion (deletes " + d.DeletedDate.Format(time.RFC3339) + ")"
	}

	out := utils.FormatMap(0, fields)

	out += "\nVersions:\n"
	if len(d.Versions) == 0 {
		out += "none\n"
	} else {
		for _, v := range d.Versions {
			id := "-"
			if v.VersionId != nil {
				id = *v.VersionId
			}
			out += fmt.Sprintf("  %s  [%s]  %s\n", id, strings.Join(v.VersionStages, ","), formatSecretsTime(v.CreatedDate))
		}
	}

	out += "\nReplication:\n"
	if len(d.Replication) == 0 {
		out += "not replicated\n"
	} else {
		for _, r := range d.Replication {
			region := "-"
			if r.Region != nil {
				region = *r.Region
			}
			out += fmt.Sprintf("  %s: %s\n", region, r.Status)
		}
	}

	out += "\nTags:\n"
	if len(d.Tags) == 0 {
		out += "none\n"
	} else {
		for _, t := range d.Tags {
			k, v := "-", "-"
			if t.Key != nil {
				k = *t.Key
			}
			if t.Value != nil {
				v = *t.Value
			}
			out += fmt.Sprintf("  %s=%s\n", k, v)
		}
	}

	out += "\nResource Policy:\n"
	if d.ResourcePolicy == "" {
		out += "not configured\n"
	} else {
		out += d.ResourcePolicy + "\n"
	}

	return out
}

func formatSecretRotation(d *aws.SecretDetails) string {
	if !d.RotationEnabled {
		return "disabled"
	}
	if d.Rotation != nil && d.Rotation.AutomaticallyAfterDays != nil {
		return fmt.Sprintf("enabled, every %d day(s)", *d.Rotation.AutomaticallyAfterDays)
	}
	return "enabled"
}

func formatSecretsTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format(time.RFC3339)
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
