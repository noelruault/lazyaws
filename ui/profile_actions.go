// Package ui routes profile refs, Enter, and menu actions through one switch path.
package ui

import (
	"context"

	"github.com/noelruault/lazyaws/ui/resources"
)

// ProfileActions keeps switching available in read-only mode because it changes no AWS state.
func (gui *Gui) ProfileActions() []resources.Action {
	profile, err := gui.Panels.Profile.GetSelectedItem()
	if err != nil {
		return nil
	}
	if profile == gui.CurrentProfile {
		return nil
	}

	return []resources.Action{{
		Name: "Switch to " + profile,
		Run: func(_ context.Context, _ string) error {
			// switchProfile runs its own waiting status and generation guard, so it is handed the profile rather than the action's context.
			return gui.switchProfile(profile)
		},
	}}
}
