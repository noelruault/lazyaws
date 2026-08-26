package presentation

import (
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/ui/utils"
)

// minTwoColWidth is the narrowest terminal that still gets two columns. Below it two columns are slivers, and the overviews read better stacked at full width.
const minTwoColWidth = 110

// Columns zips two pre-rendered blocks side by side, separated by a "│" rule with gap blank cells on each side of it.
// Each line is cut to its own column so a wide value in one block cannot push into the other. Below minTwoColWidth the right block is stacked under the left one instead, which is what makes "collapse to one column on a narrow terminal" a single policy rather than per-resource logic.
func Columns(width, gap int, left, right string) string {
	if right == "" {
		return left
	}
	gap = max(gap, 0)

	column := (width - 2*gap - 1) / 2
	if width < minTwoColWidth || column <= 0 {
		if left == "" {
			return right
		}
		return left + "\n" + right
	}

	leftLines, rightLines := strings.Split(left, "\n"), strings.Split(right, "\n")
	rule := strings.Repeat(" ", gap) + "│" + strings.Repeat(" ", gap)

	lines := make([]string, max(len(leftLines), len(rightLines)))
	for i := range lines {
		var leftLine, rightLine string
		if i < len(leftLines) {
			leftLine = truncateStyled(leftLines[i], column)
		}
		if i < len(rightLines) {
			rightLine = truncateStyled(rightLines[i], column)
		}
		lines[i] = strings.TrimRight(utils.WithPadding(leftLine, column)+rule+rightLine, " ")
	}

	return strings.Join(lines, "\n")
}

// truncateStyled cuts an already-rendered line to width terminal cells, copying its escape sequences through untouched: they carry no width, and slicing one apart bleeds its colour into the rest of the row.
// Width is measured per rune rather than per grapheme cluster, so a combining mark would be counted twice; AWS identifiers do not carry them.
func truncateStyled(line string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(utils.Decolorise(line)) <= width {
		return line
	}

	budget := width - 1 // the "…" that marks the cut needs a cell of its own
	var out strings.Builder
	var used int
	var styled bool

	for i := 0; i < len(line); {
		if length := csiPrefixLen(line[i:]); length > 0 {
			out.WriteString(line[i : i+length])
			styled = true
			i += length
			continue
		}

		r, size := utf8.DecodeRuneInString(line[i:])
		cells := runewidth.RuneWidth(r)
		if used+cells > budget {
			break
		}
		out.WriteString(line[i : i+size])
		used += cells
		i += size
	}
	out.WriteString("…")

	// The cut discards whatever reset came after it, so close the line unconditionally: a redundant reset costs nothing, an unclosed one colours the next column.
	if styled {
		out.WriteString("\x1b[0m")
	}

	return out.String()
}

// csiPrefixLen reports the byte length of the CSI escape sequence starting s, or 0 when s does not start with one.
func csiPrefixLen(s string) int {
	if !strings.HasPrefix(s, "\x1b[") {
		return 0
	}
	for i := 2; i < len(s); i++ {
		if c := s[i]; c >= '@' && c <= '~' {
			return i + 1
		}
	}
	return 0
}
