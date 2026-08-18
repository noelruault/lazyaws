package ui

import (
	"testing"

	"github.com/jesseduffield/gocui"
)

// Displayed key names must match the raw codes gocui dispatches.
func TestBindingGetKey(t *testing.T) {
	tests := []struct {
		name string
		key  interface{}
		want string
	}{
		{"rune", 'x', "x"},
		{"esc", gocui.KeyEsc, "esc"},
		{"enter", gocui.KeyEnter, "enter"},
		{"space", ' ', "space"},
		{"pgup", gocui.KeyPgup, "PgUp"},
		{"pgdn", gocui.KeyPgdn, "PgDn"},
	}

	for _, tt := range tests {
		b := &Binding{Key: tt.key}
		if got := b.GetKey(); got != tt.want {
			t.Errorf("%s: GetKey() = %q, want %q", tt.name, got, tt.want)
		}
	}
}
