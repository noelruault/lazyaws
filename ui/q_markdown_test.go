package ui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/noelruault/lazyaws/ui/utils"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
)

// A short snippet is more useful on screen than behind a click; a long one is the opposite.
func TestFoldDefaultsByBlockLength(t *testing.T) {
	short := "here you go:\n```bash\naws s3 ls\n```\nthat's it\n"
	long := "here you go:\n```bash\n" + strings.Repeat("aws s3 ls\n", qFoldThreshold+1) + "```\n"

	if folded := qFoldDefaults(short); folded[0] {
		t.Error("a one-line block starts folded, want it open")
	}
	if folded := qFoldDefaults(long); !folded[0] {
		t.Errorf("a %d-line block starts open, want it folded", qFoldThreshold+1)
	}

	if folded := qFoldDefaults("just prose\n"); len(folded) != 0 {
		t.Errorf("folds = %v, want none", folded)
	}

	// A block still arriving must not be folded on the strength of the lines that have landed so far.
	if folded := qFoldDefaults("```bash\naws s3 ls\n"); folded[0] {
		t.Error("an unfinished short block starts folded, want it open")
	}
}

// The fold state decides what the render shows, and the render must not decide it back.
func TestRenderHonoursFoldState(t *testing.T) {
	text := "```json\n" + strings.Repeat("{}\n", qFoldThreshold+1) + "```\n"

	folded := map[int]bool{0: true}
	render := renderQMarkdown(text, 80, folded)
	if strings.Contains(render.String(), "{}") {
		t.Errorf("render = %q, want the folded body hidden", render.String())
	}
	if !strings.Contains(render.String(), "click to expand") {
		t.Errorf("render = %q, want the fold to say how to open it", render.String())
	}
	if !strings.Contains(render.String(), "json") || !strings.Contains(render.String(), "9 lines") {
		t.Errorf("render = %q, want the fold to name the language and line count", render.String())
	}

	folded[0] = false
	render = renderQMarkdown(text, 80, folded)
	if !strings.Contains(render.String(), "{}") {
		t.Errorf("render = %q, want the unfolded body", render.String())
	}
	if !strings.Contains(render.String(), "click to collapse") {
		t.Errorf("render = %q, want the open block to say how to close it", render.String())
	}
	if folded[0] {
		t.Error("the render wrote back over the state the caller passed in")
	}
}

// A click arrives as a row number, so every fold has to know exactly which rows it owns.
func TestFoldAtResolvesClickedRows(t *testing.T) {
	text := "intro line\n```bash\naws s3 ls\naws s3 mb s3://x\n```\ntrailing line\n"

	render := renderQMarkdown(text, 80, map[int]bool{})
	if len(render.Folds) != 1 {
		t.Fatalf("folds = %d, want 1", len(render.Folds))
	}

	fold := render.Folds[0]
	if got := render.FoldAt(fold.FirstRow); got != 0 {
		t.Errorf("FoldAt(header row) = %d, want 0", got)
	}
	if got := render.FoldAt(fold.LastRow - 1); got != 0 {
		t.Errorf("FoldAt(last body row) = %d, want 0", got)
	}
	if got := render.FoldAt(fold.FirstRow - 1); got != -1 {
		t.Errorf("FoldAt(row above the block) = %d, want -1", got)
	}
	if got := render.FoldAt(fold.LastRow); got != -1 {
		t.Errorf("FoldAt(row below the block) = %d, want -1", got)
	}
}

func TestSeveralBlocksFoldIndependently(t *testing.T) {
	body := strings.Repeat("line\n", qFoldThreshold+1)
	text := "first:\n```bash\n" + body + "```\nsecond:\n```json\n" + body + "```\n"

	folded := qFoldDefaults(text)
	render := renderQMarkdown(text, 80, folded)
	if len(render.Folds) != 2 {
		t.Fatalf("folds = %d, want 2", len(render.Folds))
	}
	if !folded[0] || !folded[1] {
		t.Fatalf("folds = %v, want both folded to start", folded)
	}

	folded[1] = false
	render = renderQMarkdown(text, 80, folded)

	rows := render.String()
	if strings.Count(rows, "click to expand") != 1 {
		t.Errorf("render = %q, want exactly the first block still folded", rows)
	}
	if strings.Count(rows, "click to collapse") != 1 {
		t.Errorf("render = %q, want exactly the second block open", rows)
	}
	// Opening the second block must not shift which rows belong to the first.
	if render.FoldAt(render.Folds[0].FirstRow) != 0 {
		t.Error("the first block's rows moved out from under its own fold")
	}
}

// A streamed answer can be cut off mid-block, and that must still render.
func TestUnclosedCodeBlock(t *testing.T) {
	render := renderQMarkdown("here:\n```bash\naws s3 ls", 80, map[int]bool{})

	if len(render.Folds) != 1 {
		t.Fatalf("folds = %d, want the unterminated block to still be one fold", len(render.Folds))
	}
	if !strings.Contains(render.String(), "aws s3 ls") {
		t.Errorf("render = %q, want what arrived so far", render.String())
	}
}

// One rendered row must be one screen row: the click mapping is row arithmetic, so an over-wide row would offset every fold below it.
func TestEveryRowFitsTheWidth(t *testing.T) {
	forceColor(t)

	width := 40
	text := "a normal sentence that is definitely longer than forty columns wide\n" +
		"```json\n" +
		`{"Reservations":[{"Instances":[{"InstanceId":"i-0123456789abcdef0","State":{"Name":"running"}}]}]}` + "\n" +
		"```\n" +
		strings.Repeat("x", 200) + "\n"

	render := renderQMarkdown(text, width, map[int]bool{0: false})

	for i, row := range render.Rows {
		if got := runewidth.StringWidth(stripANSIForTest(row)); got > width {
			t.Errorf("row %d is %d columns wide, want at most %d: %q", i, got, width, row)
		}
	}
}

func TestInlineMarkdownStyling(t *testing.T) {
	// fatih/color drops escapes when stdout isn't a terminal, which it isn't under `go test`; the TUI always has one.
	forceColor(t)

	render := renderQMarkdown("run **now** with `aws s3 ls` or *later*\n", 200, map[int]bool{})
	row := render.Rows[0]

	for _, marker := range []string{"**", "`"} {
		if strings.Contains(row, marker) {
			t.Errorf("row = %q, want the %q markers consumed", row, marker)
		}
	}
	for _, word := range []string{"now", "aws s3 ls", "later"} {
		if !strings.Contains(row, word) {
			t.Errorf("row = %q, want it to still contain %q", row, word)
		}
	}
	if !strings.Contains(row, "\x1b[") {
		t.Errorf("row = %q, want styling escape codes", row)
	}
}

// Plain text must come through untouched: no stray escape codes, no lost characters.
func TestPlainTextIsLeftAlone(t *testing.T) {
	render := renderQMarkdown("just a plain sentence\n", 200, map[int]bool{})

	if got := render.Rows[0]; got != "just a plain sentence" {
		t.Errorf("row = %q, want it verbatim", got)
	}
	if len(render.Folds) != 0 {
		t.Errorf("folds = %d, want none", len(render.Folds))
	}
}

// styleInline runs on every prose row of every render, so it is the first place a fast path gets added and the first place one would silently change output.
func TestStyleInlineLeavesUnmarkedRowsExactlyAlone(t *testing.T) {
	forceColor(t)

	for _, row := range []string{
		"",
		"a plain sentence with no markup at all",
		"2 * 3 = 6",                       // a lone asterisk is not an italic pair
		"an unclosed `code span",          // a lone backtick is not a code span
		"snake_case_identifier stays put", // underscores are not italics here
		"a-b_c/d.e:f",
	} {
		if got := styleInline(row); got != row {
			t.Errorf("styleInline(%q) = %q, want it returned verbatim", row, got)
		}
	}
}

func TestStyleInlineAppliesEachMarker(t *testing.T) {
	forceColor(t)

	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{"bold", "say **now** ok", "say " + utils.ColoredString("now", color.Bold) + " ok"},
		{"code", "run `aws s3 ls` ok", "run " + utils.ColoredString("aws s3 ls", color.FgCyan) + " ok"},
		{"italic", "say *later* ok", "say " + utils.ColoredString("later", color.Italic) + " ok"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := styleInline(tt.in); got != tt.want {
				t.Errorf("styleInline(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Bold must be consumed before italic, or **x** degrades into an italic run of *x*.
func TestStyleInlineRunsBoldBeforeItalic(t *testing.T) {
	forceColor(t)

	got := styleInline("**strong**")
	if want := utils.ColoredString("strong", color.Bold); got != want {
		t.Errorf("styleInline(**strong**) = %q, want bold %q", got, want)
	}
	if italic := utils.ColoredString("strong", color.Italic); got == italic {
		t.Error("**strong** was styled italic; bold must win")
	}
}

func TestWrapLineSplitsByDisplayWidth(t *testing.T) {
	for _, tt := range []struct {
		name  string
		line  string
		width int
		want  []string
	}{
		{"empty keeps its screen row", "", 10, []string{""}},
		{"fits exactly", "abcde", 5, []string{"abcde"}},
		{"splits mid-word", "0123456789", 4, []string{"0123", "4567", "89"}},
		{"double-width runes cost two cells", "日本語", 4, []string{"日本", "語"}},
		{"mixed widths", "a日b", 2, []string{"a", "日", "b"}},
		{"rune wider than the budget gets its own row", "日本", 1, []string{"日", "本"}},
		{"non-positive width degrades to one cell", "abc", 0, []string{"a", "b", "c"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapLine(tt.line, tt.width)
			if len(got) != len(tt.want) {
				t.Fatalf("wrapLine(%q, %d) = %q, want %q", tt.line, tt.width, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("row %d = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// The renderer indexes fold ranges by screen row, so wrapping may never drop or reorder input.
func TestWrapLineIsLossless(t *testing.T) {
	lines := []string{"", "short", "0123456789", "日本語テキスト", "a日b c", strings.Repeat("x", 200)}

	for _, line := range lines {
		for width := 1; width <= 12; width++ {
			rows := wrapLine(line, width)
			if joined := strings.Join(rows, ""); joined != line {
				t.Fatalf("wrapLine(%q, %d) rejoined to %q", line, width, joined)
			}
			for _, row := range rows {
				// A single rune wider than the budget is the one allowed overflow.
				if runewidth.StringWidth(row) > width && utf8.RuneCountInString(row) > 1 {
					t.Errorf("wrapLine(%q, %d) row %q exceeds the width", line, width, row)
				}
			}
		}
	}
}

// forceColor is the marker for tests that depend on styled output. The actual write happens once in TestMain, before the render loop's goroutine exists: toggling the color.NoColor global per test raced a RerenderList closure a previous test left queued on the loop.
func forceColor(t *testing.T) {
	t.Helper()

	if color.NoColor {
		t.Fatal("colour is off: TestMain must force it before the loop starts, and nothing may toggle it per test")
	}
}

func stripANSIForTest(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		out.WriteByte(s[i])
	}

	return out.String()
}
