package ui

import "testing"

// cleanString feeds gocui directly, so a stray BOM or carriage return becomes a visible glyph in the panel rather than a silent no-op.
func TestCleanString(t *testing.T) {
	const bom = "\uFEFF"

	var gui Gui
	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain text untouched", "hello world", "hello world"},
		{"leading BOM stripped", bom + "hello", "hello"},
		{"BOM alone", bom, ""},
		{"BOM only stripped once", bom + bom + "hello", bom + "hello"},
		{"BOM mid-string is content, not a marker", "a" + bom + "b", "a" + bom + "b"},
		{"CRLF becomes LF", "a\r\nb", "a\nb"},
		{"lone CR is dropped", "a\rb", "ab"},
		{"BOM and CRLF together", bom + "a\r\nb", "a\nb"},
		{"shorter than a BOM", "ab", "ab"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := gui.cleanString(tt.in); got != tt.want {
				t.Errorf("cleanString(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
