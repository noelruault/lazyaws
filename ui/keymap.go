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
)

// Chord owns its description so the options bar and README cannot drift from the binding.
type Chord struct {
	Name        KeyName
	Key         any // rune | gocui.Key
	Modifier    gocui.Modifier
	Description string

	Where string
}

// DefaultKeys leaves navigation primitives fixed so configurable commands remain a focused surface.
var DefaultKeys = []Chord{
	{Name: KeyCommandBar, Key: ':', Description: "Go to a resource by name, or run a command"},
	{Name: KeyActions, Key: 'a', Description: "Open the actions menu for the focused item", Where: "every panel, and the main panel"},
	{Name: KeyFilter, Key: '/', Description: "Filter the focused list"},
	{Name: KeyRefreshPanel, Key: 'r', Description: "Refresh the focused panel"},
	{Name: KeyRefreshAll, Key: 'R', Description: "Refresh everything"},
	{Name: KeyPrevTab, Key: '[', Description: "Previous detail tab"},
	{Name: KeyNextTab, Key: ']', Description: "Next detail tab"},
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
)

// RenderKeyTable derives documentation from the binding source.
func RenderKeyTable() string {
	lines := []string{"| Key | Action |", "| --- | --- |"}

	for _, chord := range DefaultKeys {
		action := chord.Description
		if chord.Where != "" {
			action += " (" + chord.Where + ")"
		}
		lines = append(lines, fmt.Sprintf("| `%s` | %s |", describeKey(chord.Key), action))
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
func buildKeymap(overrides map[string]string) (Keymap, []error) {
	keymap := make(Keymap, len(DefaultKeys))
	known := make(map[KeyName]bool, len(DefaultKeys))
	for _, chord := range DefaultKeys {
		keymap[chord.Name] = chord
		known[chord.Name] = true
	}

	var problems []error
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
			// The literal is registered first and wins, so the rebound key would silently keep doing the old thing.
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
