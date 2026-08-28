package presentation

import (
	"fmt"
	"math"
	"sort"
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
	"Service Summary":       "▦",
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

// boxInnerPadding is the one blank cell between a box border and its content, the terminal's rendition of the mockups' px-2 cell padding.
const boxInnerPadding = 1

// boxedTableMinInner keeps a table legible when the pane is squeezed: below this the border chrome costs more than it organises, so the caller gets the borderless layout instead.
const boxedTableMinInner = 16

// boxedTablesOn keeps every overview on one table style while preserving the frameless layout as a one-line rollback.
var boxedTablesOn = true

// BoxedTable renders the mockups' bordered table: a square-cornered frame, a faint header row, a rule under it, and the body aligned by RenderTableFit.
// Square corners on purpose: the rounded set is the pane chrome, and reusing it here would make a table read as a nested view.
// width is the full width including borders; weights follow RenderTableFit's contract and must match the column count.
func BoxedTable(width int, weights []int, header []string, rows [][]utils.Cell) string {
	inner := width - 2 - 2*boxInnerPadding

	headerCells := make([]utils.Cell, len(header))
	for i, h := range header {
		headerCells[i] = utils.Cell{Text: h, Color: color.Faint}
	}
	all := append([][]utils.Cell{headerCells}, rows...)

	// One layout call over header and body together, so the header measures into the same column widths as the values under it.
	table, err := utils.RenderTableFit(all, max(inner, 1), weights)
	if err != nil {
		return utils.ColoredString(err.Error(), color.FgRed)
	}
	if !boxedTablesOn || inner < boxedTableMinInner {
		return table
	}

	lines := strings.Split(table, "\n")
	pad := strings.Repeat(" ", boxInnerPadding)
	rule := strings.Repeat("─", inner+2*boxInnerPadding)

	var out []string
	out = append(out, "┌"+rule+"┐")
	for i, line := range lines {
		out = append(out, "│"+pad+utils.WithPadding(line, inner)+pad+"│")
		if i == 0 {
			out = append(out, "├"+rule+"┤")
		}
	}
	out = append(out, "└"+rule+"┘")

	return strings.Join(out, "\n")
}

// Stat is one bordered stat card: a faint label over a value that keeps its own colour.
type Stat struct {
	Label string
	Value utils.Cell
}

// statCardsOn keeps every overview on one stat style while preserving aligned lines as a one-line rollback.
var statCardsOn = true

// StatBoxes renders stat cards side by side, each four rows tall.
// width <= 0 sizes each card to its own content and centres the text, which is the header's compact row: uniform widths looked tidier but overflowed an 80-cell pane the moment a fourth card joined, and per-card sizing is what keeps the row inside panes it cannot measure.
// width > 0 splits the width evenly and left-aligns, which is the Health cards.
func StatBoxes(width int, stats []Stat) string {
	if len(stats) == 0 {
		return ""
	}
	if !statCardsOn {
		rows := make([]kv, len(stats))
		for i, stat := range stats {
			rows[i] = kv{stat.Label, stat.Value.Rendered()}
		}
		return kvBlock(rows)
	}

	const gap = " "
	centred := width <= 0

	// Borders and gaps come off the budget first; a remainder narrower than the padding renders as an empty frame rather than a panic.
	filledInner := max((width-len(stats)*2-(len(stats)-1))/len(stats), 2*boxInnerPadding+1)
	innerOf := func(s Stat) int {
		if centred {
			return max(runewidth.StringWidth(s.Label), runewidth.StringWidth(s.Value.Text)) + 2*boxInnerPadding
		}
		return filledInner
	}

	top := make([]string, len(stats))
	labels := make([]string, len(stats))
	values := make([]string, len(stats))
	bottom := make([]string, len(stats))
	for i, s := range stats {
		inner := innerOf(s)
		fit := func(text string) string {
			return truncateStyled(text, inner-2*boxInnerPadding)
		}
		place := func(text string) string {
			if centred {
				return centerPad(text, inner)
			}
			pad := strings.Repeat(" ", boxInnerPadding)
			return pad + utils.WithPadding(text, inner-2*boxInnerPadding) + pad
		}

		rule := strings.Repeat("─", inner)
		top[i] = "┌" + rule + "┐"
		labels[i] = "│" + place(utils.ColoredString(fit(s.Label), color.Faint)) + "│"
		values[i] = "│" + place(utils.Cell{Text: fit(s.Value.Text), Color: s.Value.Color}.Rendered()) + "│"
		bottom[i] = "└" + rule + "┘"
	}

	return strings.Join([]string{
		strings.Join(top, gap),
		strings.Join(labels, gap),
		strings.Join(values, gap),
		strings.Join(bottom, gap),
	}, "\n")
}

// HeaderWithStats keeps compact cards beside a resource header when they fit and preserves the header's full-width reading order in plain mode.
func HeaderWithStats(width int, header string, cards []Stat) string {
	header = truncateBlock(header, width)
	stats := StatBoxes(0, cards)
	if stats == "" {
		return header
	}
	if !statCardsOn {
		return header + "\n" + truncateBlock(stats, width)
	}

	return mergeRightAligned(width, header, stats)
}

// centerPad pads a styled string to width cells with the slack split evenly, the odd cell going right.
func centerPad(s string, width int) string {
	slack := width - runewidth.StringWidth(utils.Decolorise(s))
	if slack <= 0 {
		return s
	}
	left := slack / 2

	return strings.Repeat(" ", left) + s + strings.Repeat(" ", slack-left)
}

// mergeRightAligned zips a right block onto the right edge of a left block, which Columns cannot do: its columns split the width, and a header's stat cards want exactly their own width, flush right.
// When the width cannot hold both, the right block stacks underneath instead of squeezing the header into slivers.
func mergeRightAligned(width int, left, right string) string {
	if right == "" {
		return left
	}

	rightLines := strings.Split(right, "\n")
	rightWidth := 0
	for _, line := range rightLines {
		rightWidth = max(rightWidth, runewidth.StringWidth(utils.Decolorise(line)))
	}

	const gap = 2
	leftBudget := width - rightWidth - gap
	// The mockups wrap the stat cards under the header at narrow widths; half the pane is the point where the header text stops being readable beside them.
	if leftBudget < width/2 {
		return left + "\n" + truncateBlock(right, width)
	}

	leftLines := strings.Split(left, "\n")
	lines := make([]string, max(len(leftLines), len(rightLines)))
	for i := range lines {
		var leftLine, rightLine string
		if i < len(leftLines) {
			leftLine = truncateStyled(leftLines[i], leftBudget)
		}
		if i < len(rightLines) {
			rightLine = rightLines[i]
		}
		lines[i] = strings.TrimRight(utils.WithPadding(leftLine, leftBudget+gap)+rightLine, " ")
	}

	return strings.Join(lines, "\n")
}

// SectionTitleWithNote is SectionTitle with a faint note pushed to the section's right edge, the mockups' "Service Summary … 1 service" row.
func SectionTitleWithNote(width int, title, note string) string {
	styled := SectionTitle(title)
	if note == "" {
		return styled
	}

	slack := width - runewidth.StringWidth(utils.Decolorise(styled)) - runewidth.StringWidth(note)
	if slack < 1 {
		return styled + " " + utils.ColoredString(note, color.Faint)
	}

	return styled + strings.Repeat(" ", slack) + utils.ColoredString(note, color.Faint)
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

// tagStyleChips flips every tag section between the mockups' bordered chips and plain "key: value" lines.
// The owner picked the plain lines off the gallery (2026-08-28); the chips stay behind this switch as the deliberate one-line way back.
var tagStyleChips = false

// tagsBody renders a tag list in the selected style. Every pane's tag section routes through here, so the style cannot drift per panel.
func tagsBody(width int, tags []kv) string {
	if tagStyleChips {
		return tagChips(width, tags)
	}

	lines := make([]string, len(tags))
	for i, tag := range tags {
		value := tag.value
		if value == "" {
			value = "none"
		}
		lines[i] = tag.label + ": " + utils.ColoredString(value, color.FgMagenta)
	}

	return strings.Join(lines, "\n")
}

// tagChips renders tags as the mockups' bordered chips, flowing left to right and wrapping onto a new chip row when the pane runs out of width.
// Each chip is content-sized (unlike StatBoxes' equal cards) because tag keys and values vary wildly, and the flow-wrap is what keeps a pane with many tags from growing one 3-row box per tag.
func tagChips(width int, tags []kv) string {
	const gap = " "
	pad := strings.Repeat(" ", boxInnerPadding)

	var lines []string
	var top, mid, bot []string
	used := 0
	flush := func() {
		if len(top) == 0 {
			return
		}
		lines = append(lines, strings.Join(top, gap), strings.Join(mid, gap), strings.Join(bot, gap))
		top, mid, bot = nil, nil, nil
		used = 0
	}

	for _, tag := range tags {
		value := tag.value
		if value == "" {
			value = "none"
		}
		styled := utils.ColoredString(tag.label+":", color.Faint) + " " + utils.ColoredString(value, color.FgMagenta)
		// A chip is never wider than the pane: the text inside is cut before the border would break the row.
		styled = truncateStyled(styled, max(width-2-2*boxInnerPadding, 1))
		inner := runewidth.StringWidth(utils.Decolorise(styled)) + 2*boxInnerPadding
		chip := inner + 2

		if used > 0 && used+len(gap)+chip > width {
			flush()
		}
		if used > 0 {
			used += len(gap)
		}
		used += chip

		rule := strings.Repeat("─", inner)
		top = append(top, "┌"+rule+"┐")
		mid = append(mid, "│"+pad+styled+pad+"│")
		bot = append(bot, "└"+rule+"┘")
	}
	flush()

	return strings.Join(lines, "\n")
}

// tagsBodyFrom is the map-shaped entry to tagsBody, sorted because Go randomizes map iteration and an unsorted tag block reshuffles itself on every re-render.
func tagsBodyFrom(width int, tags map[string]string) string {
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	rows := make([]kv, len(keys))
	for i, key := range keys {
		rows[i] = kv{key, tags[key]}
	}

	return tagsBody(width, rows)
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
