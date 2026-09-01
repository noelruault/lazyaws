// Package ui keeps bindings, help, and generated docs on one table so labels cannot drift.
package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jesseduffield/gocui"
)

// KeyName must remain stable because config overrides and generated docs address it rather than the current chord.
type KeyName string

const (
	KeyCommandBar           KeyName = "command-bar"
	KeyActions              KeyName = "actions"
	KeyFilter               KeyName = "filter"
	KeyCopyID               KeyName = "copy-id"
	KeyRefreshPanel         KeyName = "refresh-panel"
	KeyRefreshAll           KeyName = "refresh-all"
	KeyPrevTab              KeyName = "prev-tab"
	KeyNextTab              KeyName = "next-tab"
	KeyScreenModeNext       KeyName = "screen-mode-next"
	KeyScreenModePrev       KeyName = "screen-mode-prev"
	KeyOptionsMenu          KeyName = "options-menu"
	KeyHelp                 KeyName = "help"
	KeySettings             KeyName = "settings"
	KeyAmazonQ              KeyName = "amazon-q"
	KeyQuit                 KeyName = "quit"
	KeyECSExec              KeyName = "ecs-exec"
	KeyEC2Connect           KeyName = "ec2-connect"
	KeySecretsReveal        KeyName = "secrets-reveal"
	KeySecretsToggleDeleted KeyName = "secrets-toggle-deleted"
	KeySettingsEditFile     KeyName = "settings-edit-file"
	KeyChatPickModel        KeyName = "chat-pick-model"
	KeyChatNewConversation  KeyName = "chat-new-conversation"
	KeyChatToggleFolds      KeyName = "chat-toggle-folds"
	KeyRedraw               KeyName = "redraw"
	KeyNavUp                KeyName = "nav-up"
	KeyNavDown              KeyName = "nav-down"
	KeyNavLeft              KeyName = "nav-left"
	KeyNavRight             KeyName = "nav-right"
	KeyScrollMainUp         KeyName = "scroll-main-up"
	KeyScrollMainDown       KeyName = "scroll-main-down"
	KeyScrollMainPageUp     KeyName = "scroll-main-page-up"
	KeyScrollMainPageDown   KeyName = "scroll-main-page-down"
)

// Chord owns its description so the options bar and README cannot drift from the binding.
type Chord struct {
	Name        KeyName
	Key         any // rune | gocui.Key
	Modifier    gocui.Modifier
	Description string

	Where string
}

// DefaultKeys includes every configurable chord; fixed literals preserve baseline navigation when overrides break.
var DefaultKeys = []Chord{
	{Name: KeyNavUp, Key: 'k', Description: "Move up in the focused view"},
	{Name: KeyNavDown, Key: 'j', Description: "Move down in the focused view"},
	{Name: KeyNavLeft, Key: 'h', Description: "Move left in the focused view"},
	{Name: KeyNavRight, Key: 'l', Description: "Move right in the focused view"},
	{Name: KeyScrollMainUp, Key: gocui.KeyCtrlU, Description: "Scroll the main panel up"},
	{Name: KeyScrollMainDown, Key: gocui.KeyCtrlD, Description: "Scroll the main panel down"},
	{Name: KeyScrollMainPageUp, Key: gocui.KeyPgup, Description: "Scroll the main panel up"},
	{Name: KeyScrollMainPageDown, Key: gocui.KeyPgdn, Description: "Scroll the main panel down"},
	{Name: KeyCommandBar, Key: ':', Description: "Go to a resource by name, or run a command"},
	{Name: KeyActions, Key: 'a', Description: "Open the actions menu for the focused item", Where: "every panel, and the main panel"},
	{Name: KeyFilter, Key: '/', Description: "Filter the focused list"},
	{Name: KeyCopyID, Key: 'y', Description: "Show the selected item's full id / ARN, untruncated, to copy by hand", Where: "every panel, and the main panel"},
	{Name: KeyRefreshPanel, Key: 'r', Description: "Refresh the focused panel"},
	{Name: KeyRefreshAll, Key: 'R', Description: "Refresh everything"},
	// Comma and dot rather than brackets, because these are the shipped keys and the brackets are not reachable without AltGr on a Spanish, French, German, Italian or Portuguese layout.
	// The lazy preset puts lazydocker's brackets back for anyone who has them in their fingers.
	{Name: KeyPrevTab, Key: ',', Description: "Previous detail tab"},
	{Name: KeyNextTab, Key: '.', Description: "Next detail tab"},
	{Name: KeyScreenModeNext, Key: '+', Description: "Next screen-size mode (normal / half / full main)"},
	{Name: KeyScreenModePrev, Key: '_', Description: "Previous screen-size mode"},
	{Name: KeyOptionsMenu, Key: 'x', Description: "Show the keybindings for the current view"},
	{Name: KeyHelp, Key: '?', Description: "Show the keybindings for the current view"},
	{Name: KeySettings, Key: 'o', Description: "Open the Settings screen"},
	{Name: KeyAmazonQ, Key: 'A', Description: "Switch to the chat screen, when enabled"},
	{Name: KeyQuit, Key: 'q', Description: "Quit"},
	{Name: KeyRedraw, Key: gocui.KeyCtrlL, Description: "Repaint every cell, if the terminal was scrolled and the display is torn"},
	{Name: KeyECSExec, Key: 'e', Description: "Exec into the selected task's container", Where: "ECS"},
	{Name: KeyEC2Connect, Key: 'c', Description: "Connect to the instance over SSM", Where: "EC2"},
	{Name: KeySecretsReveal, Key: 'v', Description: "Reveal / mask the secret value", Where: "Secrets"},
	{Name: KeySecretsToggleDeleted, Key: 'd', Description: "Toggle showing deleted secrets", Where: "Secrets"},
	{Name: KeySettingsEditFile, Key: 'e', Description: "Open the config file in $EDITOR", Where: "Settings"},
	{Name: KeyChatPickModel, Key: gocui.KeyCtrlP, Description: "Choose the model", Where: "chat"},
	{Name: KeyChatNewConversation, Key: gocui.KeyCtrlN, Description: "Start a fresh conversation", Where: "chat"},
	{Name: KeyChatToggleFolds, Key: gocui.KeyCtrlF, Description: "Fold / unfold every code block", Where: "chat"},
}

const (
	KeyTableStart = "<!-- BEGIN GENERATED KEYS -->"
	KeyTableEnd   = "<!-- END GENERATED KEYS -->"

	PresetTableStart = "<!-- BEGIN GENERATED PRESETS -->"
	PresetTableEnd   = "<!-- END GENERATED PRESETS -->"
)

// RenderKeyTable derives documentation from the binding source.
func RenderKeyTable() string {
	lines := []string{"| Name | Default | Action |", "| --- | --- | --- |"}

	for _, chord := range DefaultKeys {
		action := chord.Description
		if chord.Where != "" {
			action += " (" + chord.Where + ")"
		}
		lines = append(lines, fmt.Sprintf("| `%s` | `%s` | %s |", chord.Name, describeKey(chord.Key), action))
	}

	return strings.Join(lines, "\n")
}

// PresetInternational is the layout DefaultKeys spells out, so it moves nothing: every key on it is reachable without AltGr, unlike the brackets a Spanish, French, German, Italian or Portuguese layout hides behind a modifier the terminal may deliver as Esc.
const PresetInternational = "international"

// PresetLazy is lazydocker's own layout, where this UI came from and where some fingers still are.
const PresetLazy = "lazy"

// ShippedPreset is the one a config gets when it names none.
// Which preset that is belongs in exactly this one line: swapping it swaps what a fresh install boots on, and nothing else in the code or the tests names a favourite.
const ShippedPreset = PresetInternational

// KeyPresets are chord overrides applied on top of DefaultKeys, before the user's own keybindings map.
// A preset holds only what it MOVES: anything it leaves out keeps the default, so a preset stays readable as the answer to "what is different here".
// Values go through gocui.Parse like any hand written override, which is what keeps a preset honest: it cannot express a chord the app could not otherwise bind, and a typo in one is reported the same way.
var KeyPresets = map[string]map[KeyName]string{
	PresetInternational: {},

	// Detail tabs back on the brackets, where lazydocker put them. Everything else already matches: hjkl and the arrows walk the panel column, Enter looks into a resource.
	PresetLazy: {
		KeyPrevTab: "[",
		KeyNextTab: "]",
	},

	// The defaults are already vim's, because lazydocker took hjkl, / and : from it. What vim does differently is the page keys: ctrl+f and ctrl+b move a screen, while ctrl+d and ctrl+u move half of one, and lazyaws had the halves without the wholes.
	// Taking ctrl+f for paging costs the chat its fold key, so that moves to ctrl+k. A preset owns the whole layout, and leaving the collision would have left one of the two dead in the chat.
	"vim": {
		KeyScrollMainPageDown: "Ctrl+F",
		KeyScrollMainPageUp:   "Ctrl+B",
		KeyChatToggleFolds:    "Ctrl+K",
	},

	// Emacs moves the cursor with ctrl+p, ctrl+n, ctrl+b and ctrl+f, pages with ctrl+v, and reverts a buffer with g, which is the convention magit and dired trained most emacs users on.
	// Two emacs keys are deliberately absent: M-v (page up) and M-x (command bar), because gocui.Parse takes Alt only with a named key, not with a letter, so neither can be expressed here. PgUp and : keep those jobs.
	// isearch's ctrl+s is absent for a different reason: terminals still use it for flow control (XOFF), so on many setups the keypress never reaches the app. Anyone who has turned that off can set filter: Ctrl+S by hand.
	// The chat's own ctrl+p and ctrl+n move aside for the cursor keys, for the same reason vim's fold key does: the cursor wins the chord it is named after in emacs, and a collision would leave one of each pair dead in the chat.
	"emacs": {
		KeyNavUp:               "Ctrl+P",
		KeyNavDown:             "Ctrl+N",
		KeyNavLeft:             "Ctrl+B",
		KeyNavRight:            "Ctrl+F",
		KeyScrollMainPageDown:  "Ctrl+V",
		KeyRefreshPanel:        "g",
		KeyRefreshAll:          "G",
		KeyChatPickModel:       "Ctrl+O",
		KeyChatNewConversation: "Ctrl+T",
	},
}

// PresetNames lists every preset by name, alphabetically, for the docs, the config comment and the Settings row.
// No name leads: which one ships is one constant, not an ordering, so adding a preset needs nothing here.
func PresetNames() []string {
	names := make([]string, 0, len(KeyPresets))
	for name := range KeyPresets {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

// RenderPresetTable derives the preset documentation from KeyPresets, so a preset gaining a key cannot leave the README describing the old one.
func RenderPresetTable() string {
	lines := []string{"| Preset | Moves |", "| --- | --- |"}

	for _, name := range PresetNames() {
		label := "`" + name + "`"

		moves := KeyPresets[name]
		if len(moves) == 0 {
			lines = append(lines, fmt.Sprintf("| %s | nothing, this is the table above |", label))
			continue
		}

		changed := make([]string, 0, len(moves))
		for _, key := range sortedPresetNames(moves) {
			changed = append(changed, fmt.Sprintf("`%s` to `%s`", key, moves[key]))
		}
		lines = append(lines, fmt.Sprintf("| %s | %s |", label, strings.Join(changed, ", ")))
	}

	return strings.Join(lines, "\n")
}

type Keymap map[KeyName]Chord

// Get falls back for hand-built test GUIs but panics on unknown names because merged configuration has already been validated.
func (k Keymap) Get(name KeyName) Chord {
	if chord, ok := k[name]; ok {
		return chord
	}

	for _, chord := range DefaultKeys {
		if chord.Name == name {
			return chord
		}
	}

	panic("ui: no key named " + string(name))
}

// buildKeymap rejects invalid overrides individually so one bad entry cannot disable every binding.
// The three layers land in this order: DefaultKeys, then the preset, then the user's own keybindings map, so a single hand written key still wins over the preset that moved it.
func buildKeymap(preset string, overrides map[string]string) (Keymap, []error) {
	keymap := make(Keymap, len(DefaultKeys))
	known := make(map[KeyName]bool, len(DefaultKeys))
	for _, chord := range DefaultKeys {
		keymap[chord.Name] = chord
		known[chord.Name] = true
	}

	var problems []error

	// A config that names no preset gets the shipped one, which is also what DefaultUserConfig carries: this covers a hand written file that sets the key to nothing.
	if preset == "" {
		preset = ShippedPreset
	}

	{
		moves, ok := KeyPresets[preset]
		if !ok {
			problems = append(problems, fmt.Errorf("keybindingPreset: %q is not a preset lazyaws has (try %s)", preset, strings.Join(PresetNames(), ", ")))
		}

		for _, name := range sortedPresetNames(moves) {
			key, modifier, err := gocui.Parse(moves[name])
			if err != nil {
				// A broken preset is this app's bug, not the user's, so it says which one rather than blaming their config.
				problems = append(problems, fmt.Errorf("keybindingPreset %q: cannot bind %q to %q: %w", preset, name, moves[name], err))
				continue
			}

			chord := keymap[name]
			chord.Key, chord.Modifier = key, modifier
			keymap[name] = chord
		}
	}

	for _, name := range sortedOverrideNames(overrides) {
		if !known[KeyName(name)] {
			problems = append(problems, fmt.Errorf("keybindings: %q is not a key lazyaws has (see the keybindings table in the README)", name))
			continue
		}

		key, modifier, err := gocui.Parse(overrides[name])
		if err != nil {
			problems = append(problems, fmt.Errorf("keybindings: cannot bind %q to %q: %w", name, overrides[name], err))
			continue
		}

		chord := keymap[KeyName(name)]
		chord.Key, chord.Modifier = key, modifier
		keymap[KeyName(name)] = chord
	}

	return keymap, problems
}

// sortedPresetNames keeps a preset's application order stable, for the same reason the overrides are sorted.
func sortedPresetNames(moves map[KeyName]string) []KeyName {
	names := make([]KeyName, 0, len(moves))
	for name := range moves {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })

	return names
}

// sortedOverrideNames keeps the error order stable, since a map range would report the same broken config differently every run.
func sortedOverrideNames(overrides map[string]string) []string {
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// checkKeyConflicts allows cross-view reuse because gocui dispatches the first matching scope.
func checkKeyConflicts(bindings []*Binding) []error {
	type scope struct {
		view     string
		key      any
		modifier gocui.Modifier
	}

	type claim struct {
		names   []KeyName
		literal bool
	}

	claims := map[scope]*claim{}
	for _, binding := range bindings {
		at := scope{view: binding.ViewName, key: binding.Key, modifier: binding.Modifier}
		if claims[at] == nil {
			claims[at] = &claim{}
		}

		if binding.Name == "" {
			claims[at].literal = true
			continue
		}
		if !containsKeyName(claims[at].names, binding.Name) {
			claims[at].names = append(claims[at].names, binding.Name)
		}
	}

	var problems []error
	for at, c := range claims {
		where := "globally"
		if at.view != "" {
			where = "in the " + at.view + " panel"
		}
		sortKeyNames(c.names)

		switch {
		case len(c.names) > 1:
			problems = append(problems, fmt.Errorf("keybindings: %s both want %s %s", strings.Join(keyNameStrings(c.names), " and "), describeKey(at.key), where))
		case len(c.names) == 1 && c.literal:
			// Dispatch order would otherwise decide whether the named chord or fixed fallback wins.
			problems = append(problems, fmt.Errorf("keybindings: %s wants %s %s, which is already how you navigate there", c.names[0], describeKey(at.key), where))
		}
	}

	sort.Slice(problems, func(i, j int) bool { return problems[i].Error() < problems[j].Error() })
	return problems
}

func containsKeyName(names []KeyName, name KeyName) bool {
	for _, existing := range names {
		if existing == name {
			return true
		}
	}
	return false
}

func sortKeyNames(names []KeyName) {
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
}

func keyNameStrings(names []KeyName) []string {
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = string(name)
	}
	return out
}

// reportStartupProblems keeps dead-binding diagnostics visible without debug logging.
func (gui *Gui) reportStartupProblems() {
	if len(gui.startupProblems) == 0 {
		return
	}

	lines := make([]string, len(gui.startupProblems))
	for i, problem := range gui.startupProblems {
		lines[i] = problem.Error()
	}

	_ = gui.createErrorPanel(strings.Join(lines, "\n"))
}

func describeKey(key any) string {
	binding := Binding{Key: key}
	return binding.GetKey()
}
