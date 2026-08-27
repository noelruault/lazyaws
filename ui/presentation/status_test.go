package presentation

import (
	"testing"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/ui/utils"
)

// Decolorise so assertions hold whether or not fatih/color is active in the test environment — we care about the glyph/word chosen, not the ANSI wrapper.
func TestStatusCell(t *testing.T) {
	cases := []struct {
		raw   string
		style StatusStyle
		want  string
	}{
		{"running", StatusStyleIcon, "▶"},
		{"ACTIVE", StatusStyleShort, "R"},
		{"available", StatusStyleLong, "available"},
		{"stopped", StatusStyleShort, "X"},
		{"PROVISIONING", StatusStyleIcon, "⟳"},
		{"DRAINING", StatusStyleShort, "S"},
		{"unhealthy", StatusStyleShort, "F"},
		{"weird-state", StatusStyleShort, "?"},
		{"weird-state", StatusStyleLong, "weird-state"},
		// VPC endpoint states are TitleCase where the rest of EC2 is lowercase, so they only resolve if the lookup keeps normalising case.
		{"Available", StatusStyleIcon, "▶"},
		{"PendingAcceptance", StatusStyleIcon, "⟳"},
		{"Partial", StatusStyleShort, "P"},
		{"Rejected", StatusStyleShort, "F"},
		{"Expired", StatusStyleShort, "F"},
		{"Deleted", StatusStyleShort, "X"},
	}
	for _, c := range cases {
		if got := utils.Decolorise(StatusCell(c.raw, c.style)); got != c.want {
			t.Errorf("StatusCell(%q, %q) = %q, want %q", c.raw, c.style, got, c.want)
		}
	}
}

func TestGetProfileDisplayCells(t *testing.T) {
	cases := []struct {
		name                          string
		profile, current, region, acc string
		want                          utils.Cell
	}{
		{"not current", "staging", "prod", "us-east-1", "123", utils.Cell{Text: "staging"}},
		{"current, no credentials yet", "prod", "prod", "", "", utils.Cell{Text: "prod \u25b8 no credentials", Color: color.Bold}},
		{"current, connected", "prod", "prod", "us-east-1", "123456789012", utils.Cell{Text: "prod \u25b8 us-east-1 \u25b8 123456789012", Color: color.Bold}},
	}
	for _, c := range cases {
		cells := GetProfileDisplayCells(c.profile, c.current, c.region, c.acc)
		if len(cells) != 1 {
			t.Fatalf("%s: got %d cells, want 1", c.name, len(cells))
		}
		if cells[0] != c.want {
			t.Errorf("%s: GetProfileDisplayCells() = %+v, want %+v", c.name, cells[0], c.want)
		}
	}
}
