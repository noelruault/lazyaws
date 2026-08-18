package ui

import "testing"

// getMessageHeight drives popup auto-sizing; wrong math means popups clip their prompt or overflow the screen.
func TestGetMessageHeight(t *testing.T) {
	gui := &Gui{}

	tests := []struct {
		name    string
		wrap    bool
		message string
		width   int
		want    int
	}{
		{"single line no wrap", false, "hello", 40, 1},
		{"multi line no wrap", false, "a\nb\nc", 40, 3},
		{"long line ignored without wrap", false, "0123456789", 4, 1},
		{"wrap splits long line", true, "0123456789", 4, 3},
		{"wrap counts each line", true, "012\n0123456789", 4, 4},
		{"empty message", true, "", 10, 1},
	}

	for _, tt := range tests {
		if got := gui.getMessageHeight(tt.wrap, tt.message, tt.width); got != tt.want {
			t.Errorf("%s: getMessageHeight(%v, %q, %d) = %d, want %d", tt.name, tt.wrap, tt.message, tt.width, got, tt.want)
		}
	}
}
