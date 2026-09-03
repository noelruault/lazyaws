package presentation

import (
	"testing"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/ui/utils"
)

// The brackets are the pill: strip the colour and it still has to read as one, because that is what a user pastes into a bug report.
func TestPillBracketsTheLabelWhateverTheColour(t *testing.T) {
	for _, preset := range []PillPreset{PillNeutral, PillSuccess, PillWarning, PillError, PillInfo} {
		if got := utils.Decolorise(Pill("read-only", preset)); got != "[read-only]" {
			t.Errorf("Pill with preset %d = %q, want %q", preset, got, "[read-only]")
		}
	}
}

// Meaning decides colour so two panels cannot disagree, which only holds if each preset really carries its own.
func TestEachPillPresetCarriesItsOwnColour(t *testing.T) {
	previous := color.NoColor
	color.NoColor = false
	t.Cleanup(func() { color.NoColor = previous })

	seen := map[string]PillPreset{}
	for _, preset := range []PillPreset{PillSuccess, PillWarning, PillError, PillInfo} {
		rendered := Pill("x", preset)
		if rendered == utils.Decolorise(rendered) {
			t.Errorf("preset %d rendered without any colour: %q", preset, rendered)
			continue
		}
		if other, clash := seen[rendered]; clash {
			t.Errorf("preset %d renders identically to preset %d, so the two meanings are indistinguishable", preset, other)
		}
		seen[rendered] = preset
	}

	// Neutral is for a label with no verdict, so colouring it would imply one.
	if got := Pill("x", PillNeutral); got != "[x]" {
		t.Errorf("Pill neutral = %q, want an uncoloured %q", got, "[x]")
	}
	if got := Pill("x", PillPreset(99)); got != "[x]" {
		t.Errorf("Pill with an unknown preset = %q, want an uncoloured %q", got, "[x]")
	}
}

// Red is reserved for the dangerous-action prompts, so a pill that sits on screen permanently must not spend it.
func TestOnlyTheErrorPillIsRed(t *testing.T) {
	previous := color.NoColor
	color.NoColor = false
	t.Cleanup(func() { color.NoColor = previous })

	red := utils.ColoredString("[x]", color.FgRed)
	for _, preset := range []PillPreset{PillNeutral, PillSuccess, PillWarning, PillInfo} {
		if Pill("x", preset) == red {
			t.Errorf("preset %d renders red, which belongs to PillError alone", preset)
		}
	}
	if Pill("x", PillError) != red {
		t.Error("PillError does not render red")
	}
}

// Empty means "nothing to show", so it must not paint a pair of brackets around nothing.
func TestAnEmptyPillRendersNothing(t *testing.T) {
	if got := Pill("", PillSuccess); got != "" {
		t.Errorf("Pill(\"\") = %q, want the empty string", got)
	}
}
