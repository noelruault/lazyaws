package panels

import "testing"

// A panel whose tab set shrinks under the held index used to read past the end of the new set: a profile switch resets the ECS drill level without touching the tab index, so the previous level's last tab indexed a shorter list and the next render panicked.
func TestCurrentMainTabSurvivesAShrinkingTabSet(t *testing.T) {
	all := []MainTab[string]{{Key: "overview"}, {Key: "config"}, {Key: "events"}, {Key: "taskdef"}}

	// A shrink to exactly the held index is the boundary the panic sat on, so it gets its own case alongside a deeper one.
	for _, width := range []int{3, 2} {
		t.Run("shrunk to "+string(rune('0'+width))+" tabs", func(t *testing.T) {
			wide := true
			state := &ContextState[string]{
				GetMainTabs: func() []MainTab[string] {
					if wide {
						return all
					}

					return all[:width]
				},
				GetItemContextCacheKey: func(item string) string { return item },
			}

			state.SetMainTabIndex(len(all) - 1)
			if got := state.GetCurrentMainTab().Key; got != "taskdef" {
				t.Fatalf("current tab = %q, want %q while the set is wide", got, "taskdef")
			}

			wide = false
			if got := state.GetCurrentMainTab().Key; got != "overview" {
				t.Errorf("current tab = %q, want the first tab %q once the set shrank", got, "overview")
			}
			// The clamp has to STICK: renderContext reads mainTabIdx straight into the view's TabIndex, so a clamp that only fixes the return value leaves gocui highlighting a tab that is not there.
			if state.mainTabIdx != 0 {
				t.Errorf("mainTabIdx = %d, want it clamped to 0 and not just worked around", state.mainTabIdx)
			}
			if got := state.GetCurrentContextKey("item"); got != "item-overview" {
				t.Errorf("context key = %q, want %q", got, "item-overview")
			}
		})
	}
}
