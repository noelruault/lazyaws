package utils

import (
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
)

func forceColor(t *testing.T) {
	t.Helper()

	previous := color.NoColor
	color.NoColor = false
	t.Cleanup(func() { color.NoColor = previous })
}

// ColoredString caches a *color.Color per attribute. A cached Color must still read the global NoColor at print time, or a colour-disabled terminal gets escape codes anyway.
func TestColoredStringHonoursNoColorAfterCaching(t *testing.T) {
	previous := color.NoColor
	t.Cleanup(func() { color.NoColor = previous })

	color.NoColor = false
	coloured := ColoredString("text", color.FgGreen)
	if !strings.Contains(coloured, "\x1b[") {
		t.Fatalf("ColoredString with colour on = %q, want escape codes", coloured)
	}

	// Same attribute, now served from the cache, with colour switched off.
	color.NoColor = true
	if plain := ColoredString("text", color.FgGreen); plain != "text" {
		t.Errorf("ColoredString with colour off = %q, want %q", plain, "text")
	}

	color.NoColor = false
	if again := ColoredString("text", color.FgGreen); again != coloured {
		t.Errorf("ColoredString after re-enabling = %q, want %q", again, coloured)
	}
}

// The precomputed escape pairs must stay byte-identical to what fatih/color produces.
// Nothing else would catch the library changing a reset code under us.
func TestColoredStringMatchesTheLibraryExactly(t *testing.T) {
	previous := color.NoColor
	t.Cleanup(func() { color.NoColor = previous })
	color.NoColor = false

	for attribute := range colorWraps {
		reference := color.New(attribute)
		reference.EnableColor()

		for _, sample := range []string{"", "x", "a longer sample string", "with\nnewline"} {
			if got, want := ColoredString(sample, attribute), reference.Sprint(sample); got != want {
				t.Errorf("ColoredString(%q, %v) = %q, want %q", sample, attribute, got, want)
			}
		}
	}
}

// An attribute outside the table must still colour correctly via the fallback.
func TestColoredStringFallsBackForUntabulatedAttributes(t *testing.T) {
	previous := color.NoColor
	t.Cleanup(func() { color.NoColor = previous })
	color.NoColor = false

	if _, tabulated := colorWraps[color.Underline]; tabulated {
		t.Skip("Underline is in the table; this test needs an attribute that is not")
	}

	reference := color.New(color.Underline)
	reference.EnableColor()
	if got, want := ColoredString("x", color.Underline), reference.Sprint("x"); got != want {
		t.Errorf("fallback = %q, want %q", got, want)
	}
}

func TestColoredStringLeavesFgWhiteAlone(t *testing.T) {
	if got := ColoredString("text", color.FgWhite); got != "text" {
		t.Errorf("ColoredString(FgWhite) = %q, want it uncoloured", got)
	}
}

// The cache is shared, and rendering can run alongside other panels; -race guards the rest.
func TestColoredStringIsConcurrencySafe(t *testing.T) {
	previous := color.NoColor
	t.Cleanup(func() { color.NoColor = previous })
	color.NoColor = false

	want := ColoredString("text", color.FgYellow)

	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 64 {
				if got := ColoredString("text", color.FgYellow); got != want {
					t.Errorf("concurrent ColoredString = %q, want %q", got, want)
					return
				}
			}
		})
	}
	wg.Wait()
}

// A column wider than its budget must be cut rather than pushing its neighbours off-screen, which is the whole reason RenderTableFit exists next to RenderTable.
func TestRenderTableFitTruncatesOverBudgetColumns(t *testing.T) {
	rows := [][]Cell{
		{{Text: "ec2"}, {Text: "a-very-long-instance-name"}, {Text: "ok"}},
		{{Text: "s3"}, {Text: "short"}, {Text: "bad"}},
	}

	got, err := RenderTableFit(rows, 20, []int{0, 1, 0})
	if err != nil {
		t.Fatalf("RenderTableFit: %v", err)
	}

	want := "ec2 a-very-long… ok\ns3  short        bad"
	if got != want {
		t.Errorf("RenderTableFit =\n%q\nwant\n%q", got, want)
	}
	for _, line := range strings.Split(got, "\n") {
		if width := runewidth.StringWidth(line); width > 20 {
			t.Errorf("line %q is %d cells wide, want at most 20", line, width)
		}
	}
}

// Cutting a wide rune in half writes a broken byte sequence to the terminal, so the cut leaves the column short instead.
func TestRenderTableFitKeepsWideRunesWhole(t *testing.T) {
	got, err := RenderTableFit([][]Cell{{{Text: "日本語テキスト"}}}, 10, []int{1})
	if err != nil {
		t.Fatalf("RenderTableFit: %v", err)
	}

	if want := "日本語テ…"; got != want {
		t.Errorf("RenderTableFit = %q, want %q", got, want)
	}
	if !utf8.ValidString(got) {
		t.Errorf("RenderTableFit = %q, want valid UTF-8", got)
	}
	if width := runewidth.StringWidth(got); width > 10 {
		t.Errorf("RenderTableFit = %q at %d cells, want at most 10", got, width)
	}
}

// Equal weights cannot divide 10 cells three ways, and dropping the remainder would leave the table narrower than the terminal every time.
func TestRenderTableFitGivesTheRemainderToTheLastFlexibleColumn(t *testing.T) {
	got, err := RenderTableFit([][]Cell{{{Text: "xxxxx"}, {Text: "yyyyy"}, {Text: "zzzzz"}}}, 12, []int{1, 1, 1})
	if err != nil {
		t.Fatalf("RenderTableFit: %v", err)
	}

	// 3 + 3 + 4 content cells plus two separators: the last column is the wider one.
	if want := "xx… yy… zzz…"; got != want {
		t.Errorf("RenderTableFit = %q, want %q", got, want)
	}
	if width := runewidth.StringWidth(got); width != 12 {
		t.Errorf("RenderTableFit = %q at %d cells, want the full 12", got, width)
	}
}

// The escapes have to wrap what is left after the cut; colouring first and truncating after would slice an escape pair apart.
func TestRenderTableFitColorsTheTruncatedTextNotTheOriginal(t *testing.T) {
	forceColor(t)

	got, err := RenderTableFit([][]Cell{{{Text: "a-long-value", Color: color.FgGreen}}}, 5, []int{1})
	if err != nil {
		t.Fatalf("RenderTableFit: %v", err)
	}

	if want := ColoredString("a-lo…", color.FgGreen); got != want {
		t.Errorf("RenderTableFit = %q, want %q", got, want)
	}
	if strings.Contains(got, "a-long-value") {
		t.Errorf("RenderTableFit = %q, want the untruncated text gone", got)
	}
	if width := runewidth.StringWidth(Decolorise(got)); width != 5 {
		t.Errorf("RenderTableFit = %q at %d visible cells, want 5", got, width)
	}
}

func TestRenderTableFitUncoloredCellCarriesNoEscapes(t *testing.T) {
	forceColor(t)

	got, err := RenderTableFit([][]Cell{{{Text: "plain"}}}, 10, []int{1})
	if err != nil {
		t.Fatalf("RenderTableFit: %v", err)
	}

	if got != "plain" {
		t.Errorf("RenderTableFit = %q, want %q", got, "plain")
	}
}

// Content-sized columns still have to fit: a terminal narrower than the natural table would otherwise wrap and corrupt the list.
func TestRenderTableFitSpendsAScarceBudgetLeftToRight(t *testing.T) {
	rows := [][]Cell{{{Text: "0123456789"}, {Text: "0123456789"}, {Text: "0123456789"}}}

	got, err := RenderTableFit(rows, 8, []int{0, 0, 0})
	if err != nil {
		t.Fatalf("RenderTableFit: %v", err)
	}

	// Budget is 8 minus two separators; the first column takes all 6 and the rest are squeezed out.
	if want := "01234…"; got != want {
		t.Errorf("RenderTableFit = %q, want %q", got, want)
	}
}

// A table narrower than the terminal must not be stretched, so weight 0 everywhere behaves like RenderTable.
func TestRenderTableFitLeavesAContentSizedTableAlone(t *testing.T) {
	got, err := RenderTableFit([][]Cell{{{Text: "a"}, {Text: "bb"}}, {{Text: "ccc"}, {Text: "d"}}}, 40, []int{0, 0})
	if err != nil {
		t.Fatalf("RenderTableFit: %v", err)
	}

	if want := "a   bb\nccc d"; got != want {
		t.Errorf("RenderTableFit = %q, want %q", got, want)
	}
}

func TestRenderTableFitAtZeroWidthRendersNothing(t *testing.T) {
	got, err := RenderTableFit([][]Cell{{{Text: "abc"}, {Text: "de"}}, {{Text: "f"}, {Text: "g"}}}, 0, []int{0, 1})
	if err != nil {
		t.Fatalf("RenderTableFit: %v", err)
	}

	if want := "\n"; got != want {
		t.Errorf("RenderTableFit = %q, want two empty lines", got)
	}
}

func TestRenderTableFitEmptyInput(t *testing.T) {
	got, err := RenderTableFit(nil, 20, nil)
	if err != nil {
		t.Fatalf("RenderTableFit: %v", err)
	}
	if got != "" {
		t.Errorf("RenderTableFit = %q, want empty", got)
	}
}

// A miscounted weight slice or a ragged row is a caller bug that would silently mis-lay-out every row, so it is an error rather than a guess.
func TestRenderTableFitRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name    string
		rows    [][]Cell
		weights []int
	}{
		{"ragged rows", [][]Cell{{{Text: "a"}, {Text: "b"}}, {{Text: "c"}}}, []int{0, 0}},
		{"too few weights", [][]Cell{{{Text: "a"}, {Text: "b"}}}, []int{0}},
		{"too many weights", [][]Cell{{{Text: "a"}}}, []int{0, 1}},
		{"negative weight", [][]Cell{{{Text: "a"}, {Text: "b"}}}, []int{0, -1}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := RenderTableFit(tc.rows, 20, tc.weights); err == nil {
				t.Errorf("RenderTableFit(%v, %v) = nil error, want one", tc.rows, tc.weights)
			}
		})
	}
}
