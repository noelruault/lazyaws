package presentation

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/ui/utils"
)

// Badge renders the state dot next to the state word, so a state is never carried by colour alone.
func Badge(status string) string {
	return BadgeCell(status).Rendered()
}

// BadgeCell is Badge for a row laid out with RenderTableFit, where the text must stay plain until the table has measured and cut it.
// The dot is fixed rather than the per-kind icon from statusStyleTable: a badge always carries the state word, so the icon's job of telling states apart is already done, and the mockups spell every badge "● word".
func BadgeCell(status string) utils.Cell {
	row := statusStyleTable[statusKindOf(status)]
	if strings.TrimSpace(status) == "" {
		return utils.Cell{Text: "●", Color: row.color}
	}
	return utils.Cell{Text: "● " + status, Color: row.color}
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

// sectionIcons carries the mockups' glyph per section, keyed on the title without any trailing count.
// A section absent here renders without an icon rather than with an invented one.
var sectionIcons = map[string]string{
	"Configuration":         "▤",
	"Details":               "▤",
	"Network":               "⇄",
	"Networking":            "⇄",
	"Networking & Security": "⌾",
	"Replication":           "⇄",
	"Gateways":              "⇄",
	"DNS":                   "⇄",
	"Metrics":               "◒",
	"Status":                "♡",
	"Health":                "♡",
	"Storage":               "▣",
	"Data management":       "▣",
	"Images":                "▣",
	"Security":              "⌾",
	"Access":                "⌾",
	"Resource policy":       "⌾",
	"Policies":              "⌾",
	"Console":               "⌘",
	"Tags":                  "◇",
	"Endpoints":             "◇",
	"Addons":                "◇",
	"Services":              "▦",
	"Subnets":               "▦",
	"Node groups":           "▦",
	"Capacity":              "⬡",
	// Not the mockups' ☷: go-runewidth counts it 2 cells while xterm draws 1, and padding built on the former misaligns every title under it.
	"Tasks":                 "≡",
	"Versions":              "≡",
	"Recent events":         "≡",
	"Control plane logging": "≡",
}

// SectionTitle renders an overview section heading, prefixed with the mockups' icon when the section has one.
func SectionTitle(s string) string {
	base := s
	// Titles can carry a count, "Tags (2)"; the icon keys on the bare name.
	if i := strings.Index(base, " ("); i > 0 {
		base = base[:i]
	}
	if icon, ok := sectionIcons[base]; ok {
		return utils.ColoredString(icon+" "+s, color.FgCyan)
	}
	return utils.ColoredString(s, color.FgCyan)
}

// ResourceHeader renders the three-line header an inspector overview opens with: the resource kind, then the name with its badge and identifier, then a meta line.
// The third line is emitted even when there is no meta, so the sections below the header do not shift up on resources that happen to carry less detail.
// resourceKindIcons carries the mockups' glyph per inspector kind; kinds the mockups do not draw reuse the closest section glyph so no header goes bare.
var resourceKindIcons = map[string]string{
	"EC2 Instance": "◇",
	"ECS Cluster":  "⬡",
	"Secret":       "▣",
	"Service":      "▦",
	"Bucket":       "▣",
	"Repository":   "⬡",
	"VPC":          "⇄",
	"EKS cluster":  "⬡",
}

func ResourceHeader(kind, name, badge, id string, meta ...string) string {
	var header strings.Builder
	if icon, ok := resourceKindIcons[kind]; ok {
		header.WriteString(utils.ColoredString(icon+" "+kind, color.FgCyan))
	} else {
		header.WriteString(utils.ColoredString(kind, color.FgCyan))
	}
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

// kv is one label/value row of an overview's key-value block.
type kv struct{ label, value string }

// kvBlock renders label/value rows with the labels padded to a common width, so the values line up in a column the eye can run down.
// Padding is applied to the coloured label because utils.WithPadding measures what is visible, not what is stored.
func kvBlock(rows []kv) string {
	label := 0
	for _, row := range rows {
		label = max(label, runewidth.StringWidth(row.label)+len(":"))
	}

	lines := make([]string, len(rows))
	for i, row := range rows {
		// Faint, not amber: the redesign spends amber on warnings and mutable state, and a label is neither.
		lines[i] = utils.WithPadding(utils.ColoredString(row.label+":", color.Faint), label) + " " + row.value
	}

	return strings.Join(lines, "\n")
}

// TagLine renders one tag row with the value in the palette's metadata colour, so every pane's tag list reads the same.
func TagLine(key, value string) string {
	if value == "" {
		value = "none"
	}
	return key + ": " + utils.ColoredString(value, color.FgMagenta)
}

// pluralize renders a count with its noun, "1 rule" / "2 rules".
func pluralize(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}

	return fmt.Sprintf("%d %ss", n, noun)
}

// fieldOr reports a failed fetch in the field's own place, for an overview whose sections each read several independent calls.
// sectionUnavailable is the right shape when one fetch feeds the whole section; this one is for a section that would otherwise throw away the lines that did answer.
func fieldOr(err error, value string) string {
	if err != nil {
		return utils.ColoredString("unavailable: "+err.Error(), color.FgRed)
	}

	return value
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
