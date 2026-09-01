package ui

import (
	"strings"
	"testing"

	"github.com/noelruault/lazyaws/ui/utils"
)

// The footer is one entry now, so the menu behind it is the only place a panel's keys are advertised: a keycap printed here that opens nothing leaves the user with no way to find any of them.
// A label naming a key the user has rebound is worse than no label, which is why nothing here hardcodes the keycap.
func TestTheFooterPointsAtTheKeyThatOpensTheMenu(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, gui.renderGlobalOptions)
	if line := utils.Decolorise(waitForView(t, g, gui.Views.Options, "Keys")); strings.TrimSpace(line) != "? Keys" {
		t.Errorf("the footer reads %q, want just the menu key", strings.TrimSpace(line))
	}

	keymap, problems := buildKeymap(ShippedPreset, map[string]string{"help": "h"})
	if len(problems) > 0 {
		t.Fatalf("rebinding help reported problems: %v", problems)
	}
	gui.Keys = keymap

	run(t, g, gui.renderGlobalOptions)
	line := utils.Decolorise(waitForView(t, g, gui.Views.Options, "h Keys"))
	if strings.Contains(line, "? Keys") {
		t.Errorf("the footer still shows the default keycap after help moved to h: %q", strings.TrimSpace(line))
	}
}
