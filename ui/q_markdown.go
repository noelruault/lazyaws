// Pre-wrapping is required because gocui does not expose the visual-row mapping needed to resolve clicks.
package ui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/ui/utils"
)

const (
	// qFoldThreshold is how many lines a code block needs before it starts folded. Short snippets are more useful on screen than behind a click.
	qFoldThreshold = 8

	qFallbackWidth = 76

	// qMinWidth guards against wrapping into a column or two while the layout settles.
	qMinWidth = 20
)

type qFold struct {
	FirstRow int
	LastRow  int
}

type qRender struct {
	Rows  []string
	Folds []qFold
}

func (r qRender) String() string {
	return strings.Join(r.Rows, "\n")
}

func (r qRender) FoldAt(row int) int {
	for i, fold := range r.Folds {
		if row >= fold.FirstRow && row < fold.LastRow {
			return i
		}
	}

	return -1
}

// renderQMarkdown leaves missing fold entries open until streaming completes.
func renderQMarkdown(text string, width int, folded map[int]bool) qRender {
	if width < qMinWidth {
		width = qFallbackWidth
	}

	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")

	// Wrapping only ever adds rows, so the line count is a floor that avoids regrowing mid-render.
	out := qRender{Rows: make([]string, 0, len(lines))}
	blockIdx := 0

	for i := 0; i < len(lines); i++ {
		lang, isFence := codeFenceInfo(lines[i])
		if !isFence {
			for _, row := range wrapLine(lines[i], width) {
				out.Rows = append(out.Rows, styleInline(row))
			}
			continue
		}

		// A streamed answer can stop mid-block, so running out of lines closes it just like a fence does.
		bodyStart := i + 1
		for i++; i < len(lines); i++ {
			if _, closing := codeFenceInfo(lines[i]); closing {
				break
			}
		}
		// Re-slicing lines keeps a folded block free: its body is never copied, only counted.
		body := lines[bodyStart:i]

		isFolded := folded[blockIdx]

		firstRow := len(out.Rows)
		out.Rows = append(out.Rows, qFoldHeader(lang, len(body), isFolded))
		if !isFolded {
			for _, line := range body {
				for _, row := range wrapLine("  "+line, width) {
					out.Rows = append(out.Rows, utils.ColoredString(row, color.FgGreen))
				}
			}
		}

		out.Folds = append(out.Folds, qFold{FirstRow: firstRow, LastRow: len(out.Rows)})
		blockIdx++
	}

	return out
}

// qFoldDefaults waits for completion because a streaming block's final length is unknown.
func qFoldDefaults(text string) map[int]bool {
	folded := map[int]bool{}

	blockIdx := 0
	inBlock := false
	bodyLines := 0

	for _, line := range strings.Split(text, "\n") {
		if _, isFence := codeFenceInfo(line); isFence {
			if inBlock {
				folded[blockIdx] = bodyLines > qFoldThreshold
				blockIdx++
				inBlock = false
				continue
			}
			inBlock = true
			bodyLines = 0
			continue
		}
		if inBlock {
			bodyLines++
		}
	}

	// An answer cut off mid-block still gets a fold, same as the renderer gives it one.
	if inBlock {
		folded[blockIdx] = bodyLines > qFoldThreshold
	}

	return folded
}

func qFoldHeader(lang string, lineCount int, folded bool) string {
	label := lang
	if label == "" {
		label = "code"
	}

	marker, hint := "▼", "[click to collapse]"
	if folded {
		marker, hint = "▶", "[click to expand]"
	}

	unit := "lines"
	if lineCount == 1 {
		unit = "line"
	}

	return utils.ColoredString(fmt.Sprintf("%s %s (%d %s) %s", marker, label, lineCount, unit, hint), color.FgYellow)
}

func codeFenceInfo(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "```") {
		return "", false
	}

	return strings.TrimSpace(strings.TrimPrefix(trimmed, "```")), true
}

var (
	qMdBold   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	qMdItalic = regexp.MustCompile(`\*([^*]+)\*`)
	qMdCode   = regexp.MustCompile("`([^`]+)`")
)

// styleInline runs after wrapping because ANSI bytes must not affect display-width arithmetic.
// styleInline runs on every prose row, where three unconditional regex passes dominated rendering; most rows carry no markers, and each pass is skipped unless its marker is present.
// Bold is consumed before italic, or **x** degrades into an italic run of *x*.
func styleInline(row string) string {
	hasStar := strings.IndexByte(row, '*') >= 0
	hasTick := strings.IndexByte(row, '`') >= 0
	if !hasStar && !hasTick {
		return row
	}

	// ReplaceAllString expands $1 directly; ReplaceAllStringFunc would hand the callback only the matched text, forcing a second match to recover the group.
	if hasStar {
		row = qMdBold.ReplaceAllString(row, utils.ColoredString("$1", color.Bold))
	}
	if hasTick {
		row = qMdCode.ReplaceAllString(row, utils.ColoredString("$1", color.FgCyan))
	}
	// Bold may have consumed every asterisk, so this is re-checked rather than reusing hasStar.
	if hasStar && strings.IndexByte(row, '*') >= 0 {
		row = qMdItalic.ReplaceAllString(row, utils.ColoredString("$1", color.Italic))
	}

	return row
}

// wrapLine splits mid-word rather than on spaces: qFold ranges index screen rows, so an over-wide row would shift every later fold boundary and misresolve clicks.
func wrapLine(line string, width int) []string {
	if line == "" {
		return []string{""}
	}
	if width < 1 {
		width = 1
	}

	var rows []string
	for start := 0; start < len(line); {
		used, end := 0, start
		for end < len(line) {
			var cost, size int
			// runewidth.RuneWidth is a table lookup behind several range checks, and terminal output is overwhelmingly ASCII.
			if c := line[end]; c < utf8.RuneSelf {
				size = 1
				if c >= 0x20 && c != 0x7f {
					cost = 1
				}
			} else {
				r, s := utf8.DecodeRuneInString(line[end:])
				size, cost = s, runewidth.RuneWidth(r)
			}

			// end > start keeps a rune wider than the whole budget on a row of its own instead of emitting an empty row forever.
			if used+cost > width && end > start {
				break
			}
			used += cost
			end += size
		}

		if rows == nil {
			// Most lines fit, and returning the input as a one-row slice skips both the append growth and the copy.
			if end == len(line) {
				return []string{line}
			}
			rows = make([]string, 0, 1+len(line)/width)
		}

		rows = append(rows, line[start:end])
		start = end
	}

	return rows
}
