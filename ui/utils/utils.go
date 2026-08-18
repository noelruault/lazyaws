// Package utils contains display helpers adapted from lazydocker (MIT, © 2018 Jesse Duffield).
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
