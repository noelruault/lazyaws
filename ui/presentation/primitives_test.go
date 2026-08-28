package presentation

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/ui/utils"
)

func forceColor(t *testing.T) {
	t.Helper()

	previous := color.NoColor
	color.NoColor = false
	t.Cleanup(func() { color.NoColor = previous })
}

func TestBadge(t *testing.T) {
	forceColor(t)

	// The dot is fixed and the colour still comes from the state kind: a badge always carries the state word, so the per-kind icon added nothing the word does not say.
	tests := []struct {
		name   string
		status string
		want   string
	}{
		{"running", "running", utils.ColoredString("● running", color.FgGreen)},
		{"alias resolves to the same colour", "ACTIVE", utils.ColoredString("● ACTIVE", color.FgGreen)},
		{"stopped", "stopped", utils.ColoredString("● stopped", color.FgRed)},
		{"pending", "creating", utils.ColoredString("● creating", color.FgYellow)},
		{"failed", "unhealthy", utils.ColoredString("● unhealthy", color.FgRed)},
		// An unknown state keeps its raw word, uncoloured, which is what says the panel could not classify it.
		{"unknown state keeps its word", "some-new-state", "● some-new-state"},
		{"no state at all is dot only", "", "●"},
		{"blank state is dot only", "   ", "●"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Badge(tc.status); got != tc.want {
				t.Errorf("Badge(%q) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}

// A badge has to be readable without colour, so the icon and the word must both survive decolorising.
func TestBadgeReadsWithoutColor(t *testing.T) {
	forceColor(t)

	if got, want := utils.Decolorise(Badge("running")), "● running"; got != want {
		t.Errorf("decolorised Badge = %q, want %q", got, want)
	}
}

func TestGauge(t *testing.T) {
	tests := []struct {
		name  string
		width int
		pct   float64
		want  string
	}{
		{"proportional fill", 10, 40, "▕████░░░░░░▏ 40.0%"},
		{"rounds to the nearest cell", 10, 12.3, "▕█░░░░░░░░░▏ 12.3%"},
		{"empty", 10, 0, "▕░░░░░░░░░░▏ 0.0%"},
		{"full", 10, 100, "▕██████████▏ 100.0%"},
		{"clamps above 100", 10, 150, "▕██████████▏ 100.0%"},
		{"clamps below 0", 10, -5, "▕░░░░░░░░░░▏ 0.0%"},
		{"zero width still reports the number", 0, 50, "▕▏ 50.0%"},
		{"negative width does not panic", -4, 50, "▕▏ 50.0%"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Gauge(tc.width, tc.pct); got != tc.want {
				t.Errorf("Gauge(%d, %v) = %q, want %q", tc.width, tc.pct, got, tc.want)
			}
		})
	}
}

// A metric with no datapoints arrives as NaN, and an unguarded NaN converts to a negative bar length and panics strings.Repeat.
func TestGaugeSurvivesNaN(t *testing.T) {
	if got, want := Gauge(10, math.NaN()), "▕░░░░░░░░░░▏ 0.0%"; got != want {
		t.Errorf("Gauge(10, NaN) = %q, want %q", got, want)
	}
}

func TestRelTime(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"seconds collapse", now.Add(-30 * time.Second), "just now"},
		{"one minute", now.Add(-time.Minute), "1m ago"},
		{"minutes", now.Add(-45 * time.Minute), "45m ago"},
		{"last minute before an hour", now.Add(-59*time.Minute - 59*time.Second), "59m ago"},
		{"one hour", now.Add(-time.Hour), "1h ago"},
		{"hours", now.Add(-6 * time.Hour), "6h ago"},
		{"last hour before a day", now.Add(-23*time.Hour - 59*time.Minute), "23h ago"},
		{"one day", now.Add(-24 * time.Hour), "1d ago"},
		{"days", now.Add(-59 * 24 * time.Hour), "59d ago"},
		{"absent timestamp", time.Time{}, "unknown"},
		{"future", now.Add(7 * 24 * time.Hour), "in 7d"},
		{"seconds into the future still collapse", now.Add(30 * time.Second), "just now"},
		{"same instant", now, "just now"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RelTime(tc.at, now); got != tc.want {
				t.Errorf("RelTime(%v, %v) = %q, want %q", tc.at, now, got, tc.want)
			}
		})
	}
}

func TestSectionTitle(t *testing.T) {
	forceColor(t)

	if got, want := SectionTitle("Configuration"), utils.ColoredString("▤ Configuration", color.FgCyan); got != want {
		t.Errorf("SectionTitle = %q, want %q", got, want)
	}
	// A count suffix must not hide the icon: the glyph keys on the bare name.
	if got, want := SectionTitle("Tags (2)"), utils.ColoredString("◇ Tags (2)", color.FgCyan); got != want {
		t.Errorf("SectionTitle with a count = %q, want %q", got, want)
	}
	// A section the mockups never drew renders bare rather than with an invented glyph.
	if got, want := SectionTitle("Bespoke"), utils.ColoredString("Bespoke", color.FgCyan); got != want {
		t.Errorf("SectionTitle without an icon = %q, want %q", got, want)
	}
}

func TestResourceHeader(t *testing.T) {
	forceColor(t)

	got := ResourceHeader("EC2 instance", "web-01", Badge("running"), "i-0abc", "t3.micro", "eu-west-1a")

	want := utils.ColoredString("EC2 instance", color.FgCyan) + "\n" +
		utils.ColoredString("web-01", color.Bold) + "  " + Badge("running") + "  " + utils.ColoredString("i-0abc", color.Faint) + "\n" +
		utils.ColoredString("t3.micro · eu-west-1a", color.Faint)
	if got != want {
		t.Errorf("ResourceHeader =\n%q\nwant\n%q", got, want)
	}
	if plain, wantPlain := utils.Decolorise(got), "EC2 instance\nweb-01  ● running  i-0abc\nt3.micro · eu-west-1a"; plain != wantPlain {
		t.Errorf("decolorised ResourceHeader =\n%q\nwant\n%q", plain, wantPlain)
	}
}

// The header keeps its three lines whatever is missing, so the sections rendered underneath do not shift up on a sparser resource.
func TestResourceHeaderStaysThreeLines(t *testing.T) {
	forceColor(t)

	tests := []struct {
		name  string
		got   string
		plain string
	}{
		{"everything", ResourceHeader("EC2 instance", "web-01", Badge("running"), "i-0abc", "t3.micro"), "EC2 instance\nweb-01  ● running  i-0abc\nt3.micro"},
		{"no meta", ResourceHeader("S3 bucket", "my-bucket", "", ""), "S3 bucket\nmy-bucket\n"},
		{"no badge", ResourceHeader("S3 bucket", "my-bucket", "", "arn:aws:s3:::my-bucket", "eu-west-1"), "S3 bucket\nmy-bucket  arn:aws:s3:::my-bucket\neu-west-1"},
		// A kind the mockups drew carries its glyph.
		{"empty meta entries are dropped, not joined", ResourceHeader("VPC", "vpc-1", "", "", "", "10.0.0.0/16", ""), "⇄ VPC\nvpc-1\n10.0.0.0/16"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if plain := utils.Decolorise(tc.got); plain != tc.plain {
				t.Errorf("decolorised header =\n%q\nwant\n%q", plain, tc.plain)
			}
			if lines := strings.Count(utils.Decolorise(tc.got), "\n") + 1; lines != 3 {
				t.Errorf("header has %d lines, want 3", lines)
			}
		})
	}
}

func TestFormatByteCount(t *testing.T) {
	tests := []struct {
		bytes float64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1.5 * 1024 * 1024 * 1024, "1.5 GiB"},
		{1024 * 1024 * 1024 * 1024, "1.0 TiB"},
	}

	for _, tc := range tests {
		if got := FormatByteCount(tc.bytes); got != tc.want {
			t.Errorf("FormatByteCount(%v) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

// The border, the rule under the header and the aligned columns are the whole point of the boxed table; each is asserted on the plain text so styling cannot hide a broken frame.
func TestBoxedTable(t *testing.T) {
	forceColor(t)

	got := utils.Decolorise(BoxedTable(40, []int{1, 0}, []string{"Name", "Status"}, [][]utils.Cell{
		{{Text: "app-service"}, {Text: "HEALTHY", Color: color.FgGreen}},
		{{Text: "checkout"}, {Text: "FAILED", Color: color.FgRed}},
	}))
	lines := strings.Split(got, "\n")

	if len(lines) != 6 {
		t.Fatalf("a two-row table renders as 6 lines (border, header, rule, rows, border), got %d:\n%s", len(lines), got)
	}
	for i, prefix := range []string{"┌", "│", "├", "│", "│", "└"} {
		if !strings.HasPrefix(lines[i], prefix) {
			t.Errorf("line %d does not open the frame with %q: %q", i, prefix, lines[i])
		}
	}
	// Every line spans the same width, or the right border zigzags.
	for i, line := range lines {
		if w := runewidth.StringWidth(line); w != 40 {
			t.Errorf("line %d is %d cells wide, want 40: %q", i, w, line)
		}
	}
	// The header and its values share one column layout.
	if strings.Index(lines[1], "Status") != strings.Index(lines[3], "HEALTHY") {
		t.Errorf("the Status column drifted between header and body:\n%s", got)
	}
}

// Below the minimum inner width the frame costs more than it organises, so the table degrades to the borderless layout rather than to crushed columns.
func TestBoxedTableDropsTheFrameWhenSqueezed(t *testing.T) {
	got := BoxedTable(12, []int{1}, []string{"Name"}, [][]utils.Cell{{{Text: "app"}}})

	if strings.Contains(got, "┌") {
		t.Errorf("a 12-cell table still spends cells on a frame:\n%s", got)
	}
	if !strings.Contains(got, "app") {
		t.Errorf("the frameless fallback lost the content:\n%s", got)
	}
}

// Compact cards size to the widest of them and centre their text; that is the header's stat row.
func TestStatBoxesCompact(t *testing.T) {
	forceColor(t)

	got := utils.Decolorise(StatBoxes(0, []Stat{
		{Label: "Services", Value: utils.Cell{Text: "1 / 1", Color: color.FgGreen}},
		{Label: "Pending", Value: utils.Cell{Text: "0"}},
	}))
	lines := strings.Split(got, "\n")

	if len(lines) != 4 {
		t.Fatalf("a stat row renders as 4 lines, got %d:\n%s", len(lines), got)
	}
	for _, want := range []string{"Services", "1 / 1", "Pending", "0"} {
		if !strings.Contains(got, want) {
			t.Errorf("the stat row lost %q:\n%s", want, got)
		}
	}
	// Two cards side by side means two frames per line.
	if strings.Count(lines[0], "┌") != 2 {
		t.Errorf("want 2 cards on the top line: %q", lines[0])
	}
	// All cards share the widest card's width, or the row's bottoms misalign.
	if len(lines[0]) != len(lines[3]) {
		t.Errorf("the frame's top and bottom differ in width:\n%s", got)
	}
}

// Filled cards split the width evenly and never overrun it; that is the Health row.
func TestStatBoxesFilledStaysInsideItsWidth(t *testing.T) {
	got := StatBoxes(60, []Stat{
		{Label: "Cluster", Value: utils.Cell{Text: "● ACTIVE"}},
		{Label: "Services", Value: utils.Cell{Text: "1 healthy"}},
		{Label: "Deployments", Value: utils.Cell{Text: "stable"}},
	})

	for _, line := range strings.Split(got, "\n") {
		if w := runewidth.StringWidth(utils.Decolorise(line)); w > 60 {
			t.Errorf("a health-card line is %d cells, over the 60-cell budget: %q", w, line)
		}
	}
	for _, want := range []string{"Cluster", "● ACTIVE", "Services", "1 healthy", "Deployments", "stable"} {
		if !strings.Contains(utils.Decolorise(got), want) {
			t.Errorf("the health cards lost %q:\n%s", want, got)
		}
	}
}

// A value wider than its card is cut inside the card rather than pushing the border out of line.
func TestStatBoxesTruncateRatherThanBreakTheFrame(t *testing.T) {
	got := utils.Decolorise(StatBoxes(30, []Stat{
		{Label: "Cluster", Value: utils.Cell{Text: "a-status-word-far-too-long-for-a-card"}},
		{Label: "Services", Value: utils.Cell{Text: "1"}},
	}))

	lines := strings.Split(got, "\n")
	first := runewidth.StringWidth(lines[0])
	for i, line := range lines {
		if w := runewidth.StringWidth(line); w != first {
			t.Errorf("line %d is %d cells wide while the frame is %d:\n%s", i, w, first, got)
		}
	}
}

// The header's stat cards sit flush right and the header text keeps the rest; on a pane too narrow for both, the cards drop underneath instead of crushing the text.
func TestMergeRightAligned(t *testing.T) {
	right := "┌───┐\n│box│\n└───┘"

	got := mergeRightAligned(60, "title line\nsecond", right)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 merged lines, got %d:\n%s", len(lines), got)
	}
	for i, wants := range [][]string{{"title line", "┌───┐"}, {"second", "│box│"}, {"└───┘"}} {
		for _, want := range wants {
			if !strings.Contains(lines[i], want) {
				t.Errorf("merged line %d lost %q: %q", i, want, lines[i])
			}
		}
		if w := runewidth.StringWidth(utils.Decolorise(lines[i])); w > 60 {
			t.Errorf("merged line %d is %d cells, over the 60-cell budget: %q", i, w, lines[i])
		}
	}
	if !strings.HasSuffix(lines[0], "┌───┐") {
		t.Errorf("the box is not flush right: %q", lines[0])
	}

	if narrow := mergeRightAligned(12, "title line", right); !strings.HasPrefix(narrow, "title line\n") {
		t.Errorf("a pane too narrow for both must stack the box under the text:\n%s", narrow)
	}
}

// The note lands on the section's right edge, and a width too small for both degrades to a single space rather than a negative repeat panic.
func TestSectionTitleWithNote(t *testing.T) {
	forceColor(t)

	got := utils.Decolorise(SectionTitleWithNote(40, "Service Summary", "1 service"))
	if !strings.HasSuffix(got, "1 service") {
		t.Errorf("the note is not at the line's end: %q", got)
	}
	if w := runewidth.StringWidth(got); w != 40 {
		t.Errorf("the titled line is %d cells, want exactly the 40-cell width: %q", w, got)
	}

	if squeezed := utils.Decolorise(SectionTitleWithNote(10, "Service Summary", "1 service")); !strings.Contains(squeezed, "Service Summary 1 service") {
		t.Errorf("a squeezed title must keep both parts: %q", squeezed)
	}
}

// Chips flow left to right and wrap onto a new 3-row chip line when the width runs out, and no line ever overruns the pane.
func TestTagChipsFlowAndWrap(t *testing.T) {
	forceColor(t)

	got := utils.Decolorise(tagChips(36, []kv{
		{"Environment", "staging"},
		{"Team", "security"},
		{"Owner", ""},
	}))
	lines := strings.Split(got, "\n")

	// 36 cells hold the first chip (24) but not the second beside it (18), so the row wraps; the second and third (15) share the next row: two chip rows of 3 lines each.
	if len(lines) != 6 {
		t.Fatalf("want 2 chip rows (6 lines), got %d:\n%s", len(lines), got)
	}
	if !strings.Contains(lines[1], "Environment: staging") {
		t.Errorf("the first chip row lost its tag:\n%s", got)
	}
	// An empty value renders as "none" rather than an empty chip.
	if !strings.Contains(lines[4], "Team: security") || !strings.Contains(lines[4], "Owner: none") {
		t.Errorf("the second chip row lost a tag:\n%s", got)
	}
	for i, line := range lines {
		if w := runewidth.StringWidth(line); w > 36 {
			t.Errorf("chip line %d is %d cells, over the 36-cell budget: %q", i, w, line)
		}
	}
}

// A tag longer than the pane is cut inside its chip rather than breaking the border.
func TestTagChipsTruncateRatherThanBreakTheFrame(t *testing.T) {
	got := utils.Decolorise(tagChips(24, []kv{{"Name", "a-value-far-too-long-for-any-chip"}}))

	for i, line := range strings.Split(got, "\n") {
		if w := runewidth.StringWidth(line); w > 24 {
			t.Errorf("chip line %d is %d cells, over the 24-cell budget: %q", i, w, line)
		}
	}
	if !strings.Contains(got, "…") {
		t.Errorf("the over-long value shows no cut mark:\n%s", got)
	}
}
