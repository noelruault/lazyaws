package presentation

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/ui/utils"
)

// Badge renders the state icon next to the state word, so a state is never carried by colour alone.
func Badge(status string) string {
	return BadgeCell(status).Rendered()
}

// BadgeCell is Badge for a row laid out with RenderTableFit, where the text must stay plain until the table has measured and cut it.
func BadgeCell(status string) utils.Cell {
	row := statusStyleTable[statusKindOf(status)]
	if strings.TrimSpace(status) == "" {
		return utils.Cell{Text: row.icon, Color: row.color}
	}
	return utils.Cell{Text: row.icon + " " + status, Color: row.color}
}

// Gauge renders a textual meter, "▕████░░░░░░▏ 40.0%". width sizes the bar body only, excluding the brackets and the number.
func Gauge(width int, pct float64) string {
	// A CloudWatch metric with no datapoints arrives as NaN, and converting NaN to int is undefined in Go: the bar length goes negative and strings.Repeat panics.
	if math.IsNaN(pct) || pct < 0 {
		pct = 0
	}
	pct = min(pct, 100)
	width = max(width, 0)

	filled := int(math.Round(pct / 100 * float64(width)))
	return fmt.Sprintf("▕%s%s▏ %.1f%%", strings.Repeat("█", filled), strings.Repeat("░", width-filled), pct)
}

// RelTime compresses an absolute time for an overview row, "6h ago" or "59d ago". Detail tabs keep the exact timestamp.
func RelTime(t, now time.Time) string {
	// An absent AWS timestamp arrives as the zero value, and rendering that relatively would claim an event in the year 1.
	if t.IsZero() {
		return "unknown"
	}

	elapsed := now.Sub(t)
	future := elapsed < 0
	if future {
		elapsed = -elapsed
	}

	if elapsed < time.Minute {
		return "just now"
	}
	if future {
		return "in " + compactDuration(elapsed)
	}
	return compactDuration(elapsed) + " ago"
}

// compactDuration renders a duration at its largest whole unit: "45m", "6h", "59d".
func compactDuration(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// SectionTitle renders an overview section heading.
func SectionTitle(s string) string {
	return utils.ColoredString(s, color.FgCyan)
}

// ResourceHeader renders the three-line header an inspector overview opens with: the resource kind, then the name with its badge and identifier, then a meta line.
// The third line is emitted even when there is no meta, so the sections below the header do not shift up on resources that happen to carry less detail.
func ResourceHeader(kind, name, badge, id string, meta ...string) string {
	var header strings.Builder
	header.WriteString(utils.ColoredString(kind, color.FgCyan))
	header.WriteString("\n")
	header.WriteString(utils.ColoredString(name, color.Bold))
	if badge != "" {
		header.WriteString("  " + badge)
	}
	if id != "" {
		header.WriteString("  " + utils.ColoredString(id, color.Faint))
	}
	header.WriteString("\n")

	// Optional AWS fields arrive empty rather than absent, and joining them anyway leaves the separator stranded between two gaps.
	populated := make([]string, 0, len(meta))
	for _, entry := range meta {
		if entry != "" {
			populated = append(populated, entry)
		}
	}
	if len(populated) > 0 {
		header.WriteString(utils.ColoredString(strings.Join(populated, " · "), color.Faint))
	}

	return header.String()
}

// FormatByteCount renders a byte count in binary units, "1.5 GiB".
func FormatByteCount(b float64) string {
	const unit = 1024.0
	if b < unit {
		return fmt.Sprintf("%.0f B", b)
	}
	div, exp := unit, 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", b/div, "KMGTPE"[exp])
}
