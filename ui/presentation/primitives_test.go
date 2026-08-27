package presentation

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"

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

	tests := []struct {
		name   string
		status string
		want   string
	}{
		{"running", "running", utils.ColoredString("▶ running", color.FgGreen)},
		{"alias resolves to the same icon and colour", "ACTIVE", utils.ColoredString("▶ ACTIVE", color.FgGreen)},
		{"stopped", "stopped", utils.ColoredString("⨯ stopped", color.FgRed)},
		{"pending", "creating", utils.ColoredString("⟳ creating", color.FgYellow)},
		{"failed", "unhealthy", utils.ColoredString("! unhealthy", color.FgRed)},
		// An unknown state keeps its raw word: the icon says the panel could not classify it, the word still says what AWS returned.
		{"unknown state keeps its word", "some-new-state", "? some-new-state"},
		{"no state at all is icon only", "", "?"},
		{"blank state is icon only", "   ", "?"},
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

	if got, want := utils.Decolorise(Badge("running")), "▶ running"; got != want {
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

	if got, want := SectionTitle("Configuration"), utils.ColoredString("Configuration", color.FgCyan); got != want {
		t.Errorf("SectionTitle = %q, want %q", got, want)
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
	if plain, wantPlain := utils.Decolorise(got), "EC2 instance\nweb-01  ▶ running  i-0abc\nt3.micro · eu-west-1a"; plain != wantPlain {
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
		{"everything", ResourceHeader("EC2 instance", "web-01", Badge("running"), "i-0abc", "t3.micro"), "EC2 instance\nweb-01  ▶ running  i-0abc\nt3.micro"},
		{"no meta", ResourceHeader("S3 bucket", "my-bucket", "", ""), "S3 bucket\nmy-bucket\n"},
		{"no badge", ResourceHeader("S3 bucket", "my-bucket", "", "arn:aws:s3:::my-bucket", "eu-west-1"), "S3 bucket\nmy-bucket  arn:aws:s3:::my-bucket\neu-west-1"},
		{"empty meta entries are dropped, not joined", ResourceHeader("VPC", "vpc-1", "", "", "", "10.0.0.0/16", ""), "VPC\nvpc-1\n10.0.0.0/16"},
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
