// Package presentation contains the pure display helpers the panels render rows and overviews with.
package presentation

import (
	"strings"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/ui/utils"
)

type StatusStyle string

const (
	StatusStyleIcon  StatusStyle = "icon"  // ▶ ⨯ ⟳ − !
	StatusStyleShort StatusStyle = "short" // R X P S F
	StatusStyleLong  StatusStyle = "long"  // the raw AWS state word (default)
)

// statusKind centralizes normalization so panels cannot invent service-specific colors.
type statusKind int

const (
	statusUnknown statusKind = iota
	statusRunning
	statusStopped
	statusPending
	statusStopping
	statusFailed
)

// statusStyleTable excludes long labels because long style preserves raw state.
var statusStyleTable = map[statusKind]struct {
	icon  string
	short string
	color color.Attribute
}{
	statusRunning:  {"▶", "R", color.FgGreen},
	statusStopped:  {"⨯", "X", color.FgRed},
	statusPending:  {"⟳", "P", color.FgYellow},
	statusStopping: {"−", "S", color.FgYellow},
	statusFailed:   {"!", "F", color.FgRed},
	statusUnknown:  {"?", "?", color.FgWhite},
}

// stateAliases centralizes service synonyms so color decisions cannot drift.
var stateAliases = map[string]statusKind{
	"running":           statusRunning,
	"active":            statusRunning,
	"available":         statusRunning,
	"healthy":           statusRunning,
	"insync":            statusRunning, // a Secrets Manager replica in step with its primary
	"inprogress":        statusPending, // a Secrets Manager replica still being created
	"completed":         statusRunning, // an ECS deployment that finished rolling out
	"in_progress":       statusPending, // an ECS deployment still rolling out
	"ok":                statusRunning, // CloudWatch alarm state
	"stopped":           statusStopped,
	"inactive":          statusStopped,
	"terminated":        statusStopped,
	"deleted":           statusStopped,
	"rejected":          statusFailed,
	"expired":           statusFailed,
	"pendingacceptance": statusPending, // a VPC endpoint awaiting the service owner's approval
	"partial":           statusPending, // a VPC endpoint up in some of its subnets only
	"pending":           statusPending,
	"provisioning":      statusPending,
	"creating":          statusPending,
	"activating":        statusPending,
	"insufficient_data": statusPending, // CloudWatch alarm state
	"stopping":          statusStopping,
	"shutting-down":     statusStopping,
	"draining":          statusStopping,
	"deleting":          statusStopping,
	"deactivating":      statusStopping,
	"failed":            statusFailed,
	"unhealthy":         statusFailed,
	"error":             statusFailed,
	"alarm":             statusFailed, // CloudWatch alarm state
}

// statusKindOf resolves a service's raw state word through the shared aliases, so a state that renders green in one panel cannot render red in another.
func statusKindOf(rawState string) statusKind {
	if kind, ok := stateAliases[strings.ToLower(strings.TrimSpace(rawState))]; ok {
		return kind
	}
	return statusUnknown
}

// StatusCellFit preserves raw text in long style and falls back to "?" for unknown compact states.
// The text stays plain so a table can measure and cut it before the colour goes on.
func StatusCellFit(rawState string, style StatusStyle) utils.Cell {
	row := statusStyleTable[statusKindOf(rawState)]

	var text string
	switch style {
	case StatusStyleIcon:
		text = row.icon
	case StatusStyleShort:
		text = row.short
	default:
		text = rawState
	}
	return utils.Cell{Text: text, Color: row.color}
}

// StatusCell is StatusCellFit already coloured, for the panels still rendering rows as strings.
func StatusCell(rawState string, style StatusStyle) string {
	cell := StatusCellFit(rawState, style)

	return utils.ColoredString(cell.Text, cell.Color)
}

// GetProfileDisplayCells adds identity only to the active profile.
func GetProfileDisplayCells(profile, currentProfile, region, accountID string) []utils.Cell {
	if profile != currentProfile {
		return []utils.Cell{{Text: profile}}
	}

	display := profile + " ▸ no credentials"
	if accountID != "" {
		display = profile + " ▸ " + region + " ▸ " + accountID
	}

	return []utils.Cell{{Text: display, Color: color.Bold}}
}
