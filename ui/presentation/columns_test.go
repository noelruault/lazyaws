package presentation

import (
	"fmt"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/ui/utils"
)

// A width well above minTwoColWidth, so tuning that constant does not rewrite every expectation below.
const (
	testWidth  = 200
	testGap    = 1
	testColumn = (testWidth - 2*testGap - 1) / 2 // 98
)

// The blocks are rendered independently and rarely have the same height, so the shorter one has to pad out and let the rule run the full length.
func TestColumnsZipsRaggedHeights(t *testing.T) {
	got := Columns(testWidth, testGap, "alpha\nbeta", "one\ntwo\nthree")

	want := strings.Join([]string{
		fmt.Sprintf("%-*s │ %s", testColumn, "alpha", "one"),
		fmt.Sprintf("%-*s │ %s", testColumn, "beta", "two"),
		fmt.Sprintf("%-*s │ %s", testColumn, "", "three"),
	}, "\n")
	if got != want {
		t.Errorf("Columns =\n%q\nwant\n%q", got, want)
	}
}

// Padding must be measured on the visible text: counting escape bytes as width would shove the rule left by however much colour the line carries.
func TestColumnsMeasuresColoredLinesWithoutTheirEscapes(t *testing.T) {
	forceColor(t)

	left := utils.ColoredString("web-01", color.FgGreen)
	got := Columns(testWidth, testGap, left, "running")

	want := left + strings.Repeat(" ", testColumn-len("web-01")) + " │ " + "running"
	if got != want {
		t.Errorf("Columns =\n%q\nwant\n%q", got, want)
	}
	if visible := runewidth.StringWidth(utils.Decolorise(got)); visible != testColumn+3+len("running") {
		t.Errorf("Columns is %d visible cells wide, want %d", visible, testColumn+3+len("running"))
	}
}

// Two columns on a narrow terminal are two unreadable slivers, so the right block goes underneath instead.
func TestColumnsStacksBelowTheTwoColumnThreshold(t *testing.T) {
	if got, want := Columns(minTwoColWidth-1, testGap, "alpha\nbeta", "one"), "alpha\nbeta\none"; got != want {
		t.Errorf("Columns = %q, want %q", got, want)
	}
	if got, want := Columns(minTwoColWidth, testGap, "alpha", "one"), fmt.Sprintf("%-*s │ %s", (minTwoColWidth-2*testGap-1)/2, "alpha", "one"); got != want {
		t.Errorf("Columns at the threshold = %q, want it zipped: %q", got, want)
	}
}

// A gap wide enough to swallow the width leaves no column to render into, so it stacks rather than emitting negative padding.
func TestColumnsStacksWhenTheGapEatsTheWidth(t *testing.T) {
	if got, want := Columns(testWidth, testWidth, "alpha", "one"), "alpha\none"; got != want {
		t.Errorf("Columns = %q, want %q", got, want)
	}
}

func TestColumnsWithAnEmptyBlock(t *testing.T) {
	if got, want := Columns(testWidth, testGap, "alpha\nbeta", ""), "alpha\nbeta"; got != want {
		t.Errorf("Columns with no right block = %q, want %q", got, want)
	}
	if got, want := Columns(testWidth, testGap, "", "one"), fmt.Sprintf("%-*s │ %s", testColumn, "", "one"); got != want {
		t.Errorf("Columns with no left block = %q, want %q", got, want)
	}
	if got, want := Columns(minTwoColWidth-1, testGap, "", "one"), "one"; got != want {
		t.Errorf("Columns stacked with no left block = %q, want %q", got, want)
	}
}

func TestColumnsTruncatesAnOverlongLine(t *testing.T) {
	got := Columns(testWidth, testGap, strings.Repeat("x", 120), "y")

	want := strings.Repeat("x", testColumn-1) + "…" + " │ " + "y"
	if got != want {
		t.Errorf("Columns =\n%q\nwant\n%q", got, want)
	}
}

// Cutting inside an escape pair is the failure this whole function exists to avoid: the opening escape must survive the cut and the line must be closed again.
func TestColumnsTruncatesWithoutBreakingEscapes(t *testing.T) {
	forceColor(t)

	long := utils.ColoredString(strings.Repeat("x", 120), color.FgGreen)
	got := Columns(testWidth, testGap, long, "y")

	cut := utils.ColoredString(strings.Repeat("x", testColumn-1), color.FgGreen)
	if wantPrefix := strings.TrimSuffix(cut, "\x1b[0m"); !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("Columns =\n%q\nwant it to open with\n%q", got, wantPrefix)
	}
	if plain, want := utils.Decolorise(strings.SplitN(got, " │ ", 2)[0]), strings.Repeat("x", testColumn-1)+"…"; plain != want {
		t.Errorf("truncated left column = %q, want %q", plain, want)
	}
	if !strings.Contains(got, "…\x1b[0m") {
		t.Errorf("Columns =\n%q\nwant the cut closed with a reset so the colour cannot bleed into the right column", got)
	}
	if visible := runewidth.StringWidth(utils.Decolorise(got)); visible != testColumn+3+1 {
		t.Errorf("Columns is %d visible cells wide, want %d", visible, testColumn+3+1)
	}
}

func TestTruncateStyled(t *testing.T) {
	forceColor(t)

	tests := []struct {
		name  string
		line  string
		width int
		want  string
	}{
		{"already fits", "abc", 10, "abc"},
		{"exactly fits", "abc", 3, "abc"},
		{"cut", "abcdef", 4, "abc…"},
		{"no room at all", "abc", 0, ""},
		{"negative width", "abc", -3, ""},
		// A wide rune cannot be halved, so the cut leaves the column a cell short rather than writing a broken sequence.
		{"wide rune at the cut", "日本語", 4, "日…"},
		{"escapes carry no width", utils.ColoredString("abc", color.FgGreen), 3, utils.ColoredString("abc", color.FgGreen)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateStyled(tc.line, tc.width)
			if got != tc.want {
				t.Errorf("truncateStyled(%q, %d) = %q, want %q", tc.line, tc.width, got, tc.want)
			}
			if width := runewidth.StringWidth(utils.Decolorise(got)); width > max(tc.width, 0) {
				t.Errorf("truncateStyled(%q, %d) = %q at %d cells, want at most %d", tc.line, tc.width, got, width, tc.width)
			}
		})
	}
}

func TestCsiPrefixLen(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"\x1b[32mabc", 5},
		{"\x1b[0m", 4},
		{"\x1b[1;32mx", 7},
		{"abc", 0},
		{"", 0},
		{"\x1b[", 0},
		{"\x1b[32", 0}, // truncated sequence with no final byte
	}

	for _, tc := range tests {
		if got := csiPrefixLen(tc.in); got != tc.want {
			t.Errorf("csiPrefixLen(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
