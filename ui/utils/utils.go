// Package utils contains the string, colour and table helpers the views render with.
package utils

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
)

func Loader() string {
	characters := "|/-\\"
	index := time.Now().UnixNano() / 50000000 % int64(len(characters))
	return characters[index : index+1]
}

func NormalizeLinefeeds(str string) string {
	str = strings.Replace(str, "\r\n", "\n", -1)
	str = strings.Replace(str, "\r", "", -1)
	return str
}

func FormatMapItem(padding int, k string, v interface{}) string {
	return fmt.Sprintf("%s%s %v\n", strings.Repeat(" ", padding), ColoredString(k+":", color.FgYellow), fmt.Sprintf("%v", v))
}

func FormatMap(padding int, m map[string]string) string {
	if len(m) == 0 {
		return "none\n"
	}

	output := "\n"

	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		output += FormatMapItem(padding, key, m[key])
	}

	return output
}

var decoloriseRe = regexp.MustCompile(`\x1B\[([0-9]{1,2}(;[0-9]{1,2})?)?[mK]`)

type colorWrap struct{ prefix, suffix string }

// colorWraps precomputes each attribute's escape pair, because color.Color.Sprint rebuilds both from its params on every call and rendering calls ColoredString once per row.
// The pairs are derived from the library rather than hand-written: fatih/color resets foreground colours with 0 rather than 39, so a hand-written table drifts silently.
var colorWraps = func() map[color.Attribute]colorWrap {
	const sentinel = "\x00"

	attributes := []color.Attribute{
		color.Bold, color.Italic,
		color.FgRed, color.FgGreen, color.FgYellow,
		color.FgBlue, color.FgMagenta, color.FgCyan,
	}

	wraps := make(map[color.Attribute]colorWrap, len(attributes))
	for _, attribute := range attributes {
		coloured := color.New(attribute)
		// EnableColor ignores the ambient NoColor so the table is still built when there is no tty.
		coloured.EnableColor()
		wrapped := coloured.Sprint(sentinel)
		at := strings.Index(wrapped, sentinel)
		wraps[attribute] = colorWrap{prefix: wrapped[:at], suffix: wrapped[at+len(sentinel):]}
	}

	return wraps
}()

// ColoredString leaves FgWhite uncoloured so light terminals remain readable.
func ColoredString(str string, colorAttribute color.Attribute) string {
	// NoColor is read per call rather than baked into the table, so a colour-disabled terminal never sees escape bytes.
	if colorAttribute == color.FgWhite || color.NoColor {
		return str
	}

	wrap, ok := colorWraps[colorAttribute]
	if !ok {
		return color.New(colorAttribute).Sprint(str)
	}

	return wrap.prefix + str + wrap.suffix
}

func OpensMenuStyle(str string) string {
	return ColoredString(fmt.Sprintf("%s...", str), color.FgMagenta)
}

func Decolorise(str string) string {
	return decoloriseRe.ReplaceAllString(str, "")
}

func WithPadding(str string, padding int) string {
	uncoloredStr := Decolorise(str)
	if padding < runewidth.StringWidth(uncoloredStr) {
		return str
	}
	return str + strings.Repeat(" ", padding-runewidth.StringWidth(uncoloredStr))
}

func RenderTable(rows [][]string) (string, error) {
	if len(rows) == 0 {
		return "", nil
	}
	if !displayArraysAligned(rows) {
		return "", errors.New("Each item must return the same number of strings to display")
	}

	columnPadWidths := getPadWidths(rows)
	paddedDisplayRows := getPaddedDisplayStrings(rows, columnPadWidths)

	return strings.Join(paddedDisplayRows, "\n"), nil
}

// Cell is one cell of a RenderTableFit table, keeping styling apart from text.
// Truncation has to happen on plain text: cutting a string that already carries ANSI escapes splits an escape pair and bleeds the colour into everything after it.
type Cell struct {
	Text string
	// Color is applied after truncation. The zero value leaves Text unstyled, since color.Reset as a cell style would mean nothing.
	Color color.Attribute
}

// Rendered is the cell's text with its own colour applied, for the tables still laid out as plain strings.
// It is the only place the zero-value rule lives: passing a 0 attribute to ColoredString would wrap the text in a reset pair rather than leave it alone.
func (c Cell) Rendered() string {
	if c.Color == 0 {
		return c.Text
	}

	return ColoredString(c.Text, c.Color)
}

// render fits the cell's text into width terminal cells and only then colours it.
func (c Cell) render(width int) string {
	// runewidth.Truncate subtracts the tail's own width from the budget, so at width 0 it still returns a one-cell "…" and overflows the column.
	if width <= 0 {
		return ""
	}

	return Cell{Text: runewidth.Truncate(c.Text, width, "…"), Color: c.Color}.Rendered()
}

// RenderTableFit lays rows out inside width terminal cells, so one long value cannot push every other column off-screen the way RenderTable does.
// A weight of 0 sizes its column to the widest cell in it; weights above 0 share what is left over in proportion, and the last of them absorbs the rounding remainder.
// Cells too wide for their column are cut with a trailing "…".
func RenderTableFit(rows [][]Cell, width int, weights []int) (string, error) {
	if len(rows) == 0 {
		return "", nil
	}

	columns := len(rows[0])
	for _, cells := range rows {
		if len(cells) != columns {
			return "", errors.New("each row must have the same number of cells to display")
		}
	}
	if len(weights) != columns {
		return "", fmt.Errorf("got %d column weights for %d columns", len(weights), columns)
	}
	for _, weight := range weights {
		if weight < 0 {
			return "", fmt.Errorf("column weight %d is negative", weight)
		}
	}

	columnWidths := fitColumnWidths(rows, width, weights)

	lines := make([]string, len(rows))
	for i, cells := range rows {
		var line strings.Builder
		for j, cell := range cells {
			if j > 0 {
				line.WriteByte(' ')
			}
			line.WriteString(WithPadding(cell.render(columnWidths[j]), columnWidths[j]))
		}
		// Columns squeezed to nothing would otherwise leave a run of separators hanging off the end of the row.
		lines[i] = strings.TrimRight(line.String(), " ")
	}

	return strings.Join(lines, "\n"), nil
}

// flexibleFloor is the width a flexible column claims before the content-sized ones are paid, enough for a recognisable prefix and an ellipsis.
// Like minTwoColWidth it is a tune-by-eye number, not a derived one.
const flexibleFloor = 8

// fitColumnWidths splits width between the columns, leaving one space between each pair.
func fitColumnWidths(rows [][]Cell, width int, weights []int) []int {
	columns := len(weights)
	widths := make([]int, columns)
	if columns == 0 {
		return widths
	}

	budget := max(width-(columns-1), 0)

	contentWidth, totalWeight, lastFlexible, flexibleCount := 0, 0, -1, 0
	for i, weight := range weights {
		if weight > 0 {
			totalWeight += weight
			lastFlexible = i
			flexibleCount++
			continue
		}
		for _, cells := range rows {
			widths[i] = max(widths[i], runewidth.StringWidth(cells[i].Text))
		}
		contentWidth += widths[i]
	}

	// Content-sized columns can fill the budget on their own, which would leave the flexible columns nothing.
	// They are the ones holding the row's identifying text, so starving them empties the column you read the list by while four narrower ones survive.
	// Claiming a floor here is what lets the left-to-right clamp below take those cells back off the trailing columns instead.
	share, given := min(max(budget-contentWidth, flexibleFloor*flexibleCount), budget), 0
	for i, weight := range weights {
		if weight == 0 {
			continue
		}
		if i == lastFlexible {
			widths[i] = share - given
			continue
		}
		widths[i] = share * weight / totalWeight
		given += widths[i]
	}

	// Content-sized columns can overrun the terminal on their own, so spend the budget left to right: the leftmost columns carry the identifying text and are the ones worth keeping.
	spent := 0
	for i := range widths {
		widths[i] = max(min(widths[i], budget-spent), 0)
		spent += widths[i]
	}

	return widths
}

func getPadWidths(rows [][]string) []int {
	if len(rows[0]) <= 1 {
		return []int{}
	}
	columnPadWidths := make([]int, len(rows[0])-1)
	for i := range columnPadWidths {
		for _, cells := range rows {
			uncoloredCell := Decolorise(cells[i])

			if runewidth.StringWidth(uncoloredCell) > columnPadWidths[i] {
				columnPadWidths[i] = runewidth.StringWidth(uncoloredCell)
			}
		}
	}
	return columnPadWidths
}

func getPaddedDisplayStrings(rows [][]string, columnPadWidths []int) []string {
	paddedDisplayRows := make([]string, len(rows))
	for i, cells := range rows {
		for j, columnPadWidth := range columnPadWidths {
			paddedDisplayRows[i] += WithPadding(cells[j], columnPadWidth) + " "
		}
		paddedDisplayRows[i] += cells[len(columnPadWidths)]
	}
	return paddedDisplayRows
}

func displayArraysAligned(stringArrays [][]string) bool {
	for _, cells := range stringArrays {
		if len(cells) != len(stringArrays[0]) {
			return false
		}
	}
	return true
}
