package presentation

import (
	"testing"

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

func TestGetProfileDisplayStrings(t *testing.T) {
	cases := []struct {
		name                          string
		profile, current, region, acc string
		want                          string
	}{
		{"not current", "staging", "prod", "us-east-1", "123", "staging"},
		{"current, no credentials yet", "prod", "prod", "", "", "prod ▸ no credentials"},
		{"current, connected", "prod", "prod", "us-east-1", "123456789012", "prod ▸ us-east-1 ▸ 123456789012"},
	}
	for _, c := range cases {
		cells := GetProfileDisplayStrings(c.profile, c.current, c.region, c.acc)
		if len(cells) != 1 {
			t.Fatalf("%s: got %d cells, want 1", c.name, len(cells))
		}
		if got := utils.Decolorise(cells[0]); got != c.want {
			t.Errorf("%s: GetProfileDisplayStrings() = %q, want %q", c.name, got, c.want)
		}
	}
}
