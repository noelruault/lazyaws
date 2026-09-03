package presentation

import (
	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/ui/utils"
)

// PillPreset is what a pill means, not what colour it is: a caller says "this is a caution" and the palette lives here, so two pills of the same meaning cannot end up different colours in two panels.
type PillPreset int

const (
	PillNeutral PillPreset = iota // no colour, for a label that carries no verdict
	PillSuccess                   // the safe or healthy state
	PillWarning                   // a caution worth noticing, not an error
	PillError                     // something is wrong now
	PillInfo                      // a fact worth showing, with no judgement attached
)

// pillColors is the whole palette. Red belongs to PillError alone, because the dangerous-action prompts are red and a red pill sitting permanently in the footer would spend that signal.
var pillColors = map[PillPreset]color.Attribute{
	PillSuccess: color.FgGreen,
	PillWarning: color.FgYellow,
	PillError:   color.FgRed,
	PillInfo:    color.FgCyan,
}

// Pill renders a bracketed label for a mode or a property of the session, "[read-only]".
// An AWS state belongs in Badge, which colours itself from the state word, and an AWS resource tag in tagChips; a Pill is told what it means.
// An unknown preset renders uncoloured rather than borrowing a colour, so a pill from a caller this package has not met stays legible.
func Pill(text string, preset PillPreset) string {
	if text == "" {
		return ""
	}

	label := "[" + text + "]"
	attr, ok := pillColors[preset]
	if !ok {
		return label
	}

	return utils.ColoredString(label, attr)
}
