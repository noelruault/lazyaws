package ui

import (
	"flag"
	"os"
	"path/filepath"
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
	start := strings.Index(readme, KeyTableStart)
	end := strings.Index(readme, KeyTableEnd)
	if start < 0 || end < 0 || end < start {
		return "", os.ErrNotExist
	}

	return readme[:start+len(KeyTableStart)] + "\n" + table + "\n" + readme[end:], nil
}

func TestBuildKeymapAppliesOverrides(t *testing.T) {
	keymap, problems := buildKeymap(map[string]string{
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
	keymap, problems := buildKeymap(map[string]string{
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
		if binding.Description != chord.Description {
			t.Errorf("%q on %q describes itself as %q, but the keymap says %q", binding.Name, binding.ViewName, binding.Description, chord.Description)
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
