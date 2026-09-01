package ui

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/config"
)

var updateDocs = flag.Bool("update", false, "rewrite the generated keybindings table in README.md")

// Generated key documentation must match the binding table; `make keys` repairs drift.
func TestReadmeKeyTableIsCurrent(t *testing.T) {
	path := filepath.Join("..", "README.md")

	readme, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}

	updated, err := spliceKeyTable(string(readme), RenderKeyTable())
	if err != nil {
		t.Fatalf("%v", err)
	}

	updated, err = spliceBlock(updated, PresetTableStart, PresetTableEnd, RenderPresetTable())
	if err != nil {
		t.Fatalf("%v", err)
	}

	if *updateDocs {
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			t.Fatalf("writing README.md: %v", err)
		}
		return
	}

	if updated != string(readme) {
		t.Error("the keybindings table in README.md no longer matches DefaultKeys; run: make keys")
	}
}

func spliceKeyTable(readme, table string) (string, error) {
	return spliceBlock(readme, KeyTableStart, KeyTableEnd, table)
}

func spliceBlock(readme, startMarker, endMarker, table string) (string, error) {
	start := strings.Index(readme, startMarker)
	end := strings.Index(readme, endMarker)
	if start < 0 || end < 0 || end < start {
		return "", os.ErrNotExist
	}

	return readme[:start+len(startMarker)] + "\n" + table + "\n" + readme[end:], nil
}

func TestBuildKeymapAppliesOverrides(t *testing.T) {
	keymap, problems := buildKeymap(ShippedPreset, map[string]string{
		"actions":         "m",
		"amazon-q":        "a",
		"chat-pick-model": "ctrl+k",
		"nav-down":        "n",
	})
	if len(problems) != 0 {
		t.Fatalf("valid overrides reported problems: %v", problems)
	}

	if got := keymap.Get(KeyActions).Key; got != 'm' {
		t.Errorf("actions = %v, want 'm'", got)
	}
	if got := keymap.Get(KeyAmazonQ).Key; got != 'a' {
		t.Errorf("amazon-q = %v, want 'a'", got)
	}
	if got := keymap.Get(KeyChatPickModel).Key; got != gocui.KeyCtrlK {
		t.Errorf("chat-pick-model = %v, want ctrl+k", got)
	}
	if got := keymap.Get(KeyNavDown).Key; got != 'n' {
		t.Errorf("nav-down = %v, want 'n'", got)
	}

	// Descriptions follow names so rebinding cannot stale help text.
	if got := keymap.Get(KeyActions).Description; got == "" {
		t.Error("rebinding actions dropped its description")
	}

	if got := keymap.Get(KeyFilter).Key; got != '/' {
		t.Errorf("filter = %v, want '/'", got)
	}
}

// One invalid override must not disable valid bindings.
func TestBuildKeymapReportsBadOverridesWithoutLosingTheRest(t *testing.T) {
	keymap, problems := buildKeymap(ShippedPreset, map[string]string{
		"actions":     "m",
		"no-such-key": "z",
		"filter":      "ctrl+nonsense",
	})

	if len(problems) != 2 {
		t.Fatalf("got %d problems, want 2: %v", len(problems), problems)
	}
	if got := keymap.Get(KeyActions).Key; got != 'm' {
		t.Errorf("a bad line cost a good one: actions = %v", got)
	}
	if got := keymap.Get(KeyFilter).Key; got != '/' {
		t.Errorf("an unparseable override should leave the default in place, got %v", got)
	}
}

// Duplicate named keys in one scope would leave the later command unreachable.
func TestNoKeyConflictsInTheDefaults(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	if problems := checkKeyConflicts(gui.GetInitialKeybindings()); len(problems) > 0 {
		t.Fatalf("the shipped keymap conflicts with itself: %v", problems)
	}
	if len(gui.startupProblems) > 0 {
		t.Fatalf("a default install reports startup problems: %v", gui.startupProblems)
	}
}

// A preset is shipped configuration, so a chord that does not parse or that lands on another key is this app's bug and has to fail here rather than on a user's first keypress.
func TestEveryPresetIsValidAndConflictFree(t *testing.T) {
	for _, preset := range PresetNames() {
		t.Run(preset, func(t *testing.T) {
			user := config.DefaultUserConfig()
			user.KeybindingPreset = preset
			gui, _ := newHeadlessGuiWithConfig(t, user)

			if len(gui.startupProblems) > 0 {
				t.Fatalf("preset %q reports startup problems: %v", preset, gui.startupProblems)
			}
			if problems := checkKeyConflicts(gui.GetInitialKeybindings()); len(problems) > 0 {
				t.Fatalf("preset %q conflicts with itself: %v", preset, problems)
			}

			// Every key a preset moves has to land, and every key it leaves alone has to stay where DefaultKeys put it. Both halves are read from the preset itself, so a new one is covered the moment it is added to KeyPresets.
			keymap, problems := buildKeymap(preset, nil)
			if len(problems) > 0 {
				t.Fatalf("preset %q reports problems: %v", preset, problems)
			}

			moves := KeyPresets[preset]
			for name, chord := range moves {
				wantKey, wantModifier := gocui.MustParse(chord)
				got := keymap.Get(name)
				if got.Key != wantKey || got.Modifier != wantModifier {
					t.Errorf("%q declares %s as %q, but the keymap has %v", preset, name, chord, got.Key)
				}
			}

			for _, chord := range DefaultKeys {
				if _, moved := moves[chord.Name]; moved {
					continue
				}
				if got := keymap.Get(chord.Name); got.Key != chord.Key || got.Modifier != chord.Modifier {
					t.Errorf("%q does not declare %s, but it changed from %v to %v", preset, chord.Name, chord.Key, got.Key)
				}
			}
		})
	}
}

// Which preset ships is one constant, and this is what keeps the three places that depend on it in agreement: the config's own value, an empty value, and KeyPresets.
// Nothing here names a layout, so changing ShippedPreset needs no edit in this file.
func TestTheShippedPresetIsRealAndIsWhatAnEmptyConfigGets(t *testing.T) {
	if _, ok := KeyPresets[ShippedPreset]; !ok {
		t.Fatalf("ShippedPreset is %q, which is not in KeyPresets (%v)", ShippedPreset, PresetNames())
	}
	if got := config.DefaultUserConfig().KeybindingPreset; got != ShippedPreset {
		t.Errorf("a fresh config asks for preset %q, want the shipped %q", got, ShippedPreset)
	}

	// Empty is what a hand written file leaves behind when it sets the key to nothing, and it has to resolve to the same layout rather than to bare DefaultKeys.
	shipped, problems := buildKeymap(ShippedPreset, nil)
	if len(problems) > 0 {
		t.Fatalf("the shipped preset reports problems: %v", problems)
	}
	empty, problems := buildKeymap("", nil)
	if len(problems) > 0 {
		t.Fatalf("an empty preset reports problems: %v", problems)
	}

	for _, chord := range DefaultKeys {
		if got, want := empty.Get(chord.Name), shipped.Get(chord.Name); got.Key != want.Key || got.Modifier != want.Modifier {
			t.Errorf("with no preset named, %s is %v, want the shipped %v", chord.Name, got.Key, want.Key)
		}
	}
}

// The one product promise worth pinning by name: whatever ships, the keys lazydocker used stay one line of config away.
func TestTheLazyPresetKeepsLazydockersBracketsReachable(t *testing.T) {
	keymap, problems := buildKeymap(PresetLazy, nil)
	if len(problems) > 0 {
		t.Fatalf("the lazy preset reports problems: %v", problems)
	}

	if got := keymap.Get(KeyPrevTab).Key; got != '[' {
		t.Errorf("prev-tab = %v, want [", got)
	}
	if got := keymap.Get(KeyNextTab).Key; got != ']' {
		t.Errorf("next-tab = %v, want ]", got)
	}
}

// The Settings row is the discoverable half of the feature, so it has to cycle the real preset list and write the name the next startup reads.
func TestTheSettingsRowCyclesThePresetsAndWritesTheChoice(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	gui, g := newHeadlessGui(t)
	run(t, g, gui.handleToggleSettings)
	selectSettingNamed(t, g, gui, "Keybinding preset")

	run(t, g, gui.handleSettingsToggle)

	// Any other name from the list is a pass: the row's job is to cycle the real presets, not to land on a particular one.
	chosen := ask(g, func() string { return gui.Config.User.KeybindingPreset })
	if chosen == ShippedPreset || !slices.Contains(PresetNames(), chosen) {
		t.Fatalf("the row cycled to %q, want another preset from %v", chosen, PresetNames())
	}

	written, err := os.ReadFile(config.ConfigFilename())
	if err != nil {
		t.Fatalf("reading the written config: %v", err)
	}
	if !strings.Contains(string(written), "keybindingPreset: "+chosen) {
		t.Errorf("config.yml does not carry the chosen preset:\n%s", written)
	}
}

// The three layers have to land in order, or a preset would silently undo a key the user set by hand.
// Every preset that moves at least one key is exercised, so a new layout gets this coverage without a line being added here.
func TestAHandWrittenKeyBeatsThePresetThatMovedIt(t *testing.T) {
	for _, preset := range PresetNames() {
		moves := KeyPresets[preset]
		if len(moves) == 0 {
			continue
		}

		t.Run(preset, func(t *testing.T) {
			// The first key the preset moves is the one taken back by hand; the rest of the preset has to survive that.
			taken := sortedPresetNames(moves)[0]
			keymap, problems := buildKeymap(preset, map[string]string{string(taken): "F1"})
			if len(problems) > 0 {
				t.Fatalf("unexpected problems: %v", problems)
			}

			if got := keymap.Get(taken).Key; got != gocui.KeyF1 {
				t.Errorf("%s = %v, want the hand written F1 to beat the preset's %q", taken, got, moves[taken])
			}

			for name, chord := range moves {
				if name == taken {
					continue
				}
				wantKey, wantModifier := gocui.MustParse(chord)
				if got := keymap.Get(name); got.Key != wantKey || got.Modifier != wantModifier {
					t.Errorf("%s = %v, want the preset's %q to still apply", name, got.Key, chord)
				}
			}
		})
	}
}

// A misspelled preset has to say so and name the real ones, and leave a working keyboard behind rather than half a layout.
func TestAnUnknownPresetIsReportedAndChangesNothing(t *testing.T) {
	keymap, problems := buildKeymap("vscode", nil)
	if len(problems) != 1 {
		t.Fatalf("problems = %v, want exactly one naming the preset", problems)
	}
	if !strings.Contains(problems[0].Error(), "vscode") {
		t.Errorf("the problem does not name the preset that does not exist: %v", problems[0])
	}
	for _, name := range PresetNames() {
		if !strings.Contains(problems[0].Error(), name) {
			t.Errorf("the problem does not offer %q as an alternative: %v", name, problems[0])
		}
	}

	for _, chord := range DefaultKeys {
		if got := keymap.Get(chord.Name); got.Key != chord.Key || got.Modifier != chord.Modifier {
			t.Errorf("%s moved to %v with an unusable preset, want the shipped %v", chord.Name, got.Key, chord.Key)
		}
	}
}

func TestCheckKeyConflictsCatchesARebindLandingOnAnotherKey(t *testing.T) {
	clash := []*Binding{
		{ViewName: "", Name: KeyActions, Key: 'r'},
		{ViewName: "", Name: KeyRefreshPanel, Key: 'r'},
	}
	problems := checkKeyConflicts(clash)
	if len(problems) != 1 {
		t.Fatalf("got %d problems, want 1: %v", len(problems), problems)
	}
	if !strings.Contains(problems[0].Error(), "actions") || !strings.Contains(problems[0].Error(), "refresh-panel") {
		t.Errorf("the complaint does not name both keys: %v", problems[0])
	}

	// The same key is valid in disjoint views.
	fine := []*Binding{
		{ViewName: "ecs", Name: KeyECSExec, Key: 'e'},
		{ViewName: "settings", Name: KeySettingsEditFile, Key: 'e'},
	}
	if problems := checkKeyConflicts(fine); len(problems) > 0 {
		t.Errorf("two panels sharing a key were reported as a conflict: %v", problems)
	}

	// gocui intentionally gives view-specific bindings precedence over global ones.
	shadow := []*Binding{
		{ViewName: "", Name: KeyQuit, Key: 'q'},
		{ViewName: "menu", Key: 'q'},
	}
	if problems := checkKeyConflicts(shadow); len(problems) > 0 {
		t.Errorf("a menu key shadowing a global was reported as a conflict: %v", problems)
	}
}

func TestNavigationRebindConflictingWithAnArrowReportsAtStartup(t *testing.T) {
	user := config.DefaultUserConfig()
	user.Keybindings = map[string]string{"nav-down": "arrow+down"}

	gui, _ := newHeadlessGuiWithConfig(t, user)
	for _, problem := range gui.startupProblems {
		if strings.Contains(problem.Error(), "nav-down wants ▼") && strings.Contains(problem.Error(), "already how you navigate there") {
			return
		}
	}

	t.Fatalf("startup problems do not reject nav-down on the fixed down arrow: %v", gui.startupProblems)
}

// Every documented named key must reach a handler.
func TestEveryNamedKeyIsBound(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	bound := map[KeyName]bool{}
	for _, binding := range gui.GetInitialKeybindings() {
		if binding.Name != "" {
			bound[binding.Name] = true
		}
	}

	for _, chord := range DefaultKeys {
		if !bound[chord.Name] {
			t.Errorf("%q is in DefaultKeys and in the README, but nothing binds it", chord.Name)
		}
	}
}

func TestNamedBindingsCarryTheirChord(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	for _, binding := range gui.GetInitialKeybindings() {
		if binding.Name == "" {
			continue
		}
		chord := gui.Keys.Get(binding.Name)
		if binding.Key != chord.Key || binding.Modifier != chord.Modifier {
			t.Errorf("%q on %q is bound to %v, but the keymap says %v", binding.Name, binding.ViewName, binding.Key, chord.Key)
		}
		if binding.Description == "" {
			t.Errorf("%q on %q carries no description, so the menu cannot list it", binding.Name, binding.ViewName)
		}

		// A global key means one thing everywhere, so its description has to be the keymap's own, which is also what the generated README table prints.
		// A view scoped binding may say what the key does THERE instead: nav-right leaves a list for the main panel and scrolls once inside it, and one sentence covering both is worse in both places.
		if binding.ViewName == "" && binding.Description != chord.Description {
			t.Errorf("global %q describes itself as %q, but the keymap says %q", binding.Name, binding.Description, chord.Description)
		}
	}
}

func TestGetKeyRendersControlKeys(t *testing.T) {
	for _, tc := range []struct {
		key  any
		want string
	}{
		{'a', "a"},
		{'A', "A"},
		{':', ":"},
		{gocui.KeyCtrlP, "ctrl+p"},
		{gocui.KeyEsc, "esc"},
		{gocui.KeyEnter, "enter"},
		{gocui.KeyTab, "tab"},
		{gocui.KeySpace, "space"},
		{nil, ""},
	} {
		binding := Binding{Key: tc.key}
		if got := binding.GetKey(); got != tc.want {
			t.Errorf("GetKey(%v) = %q, want %q", tc.key, got, tc.want)
		}
	}
}
