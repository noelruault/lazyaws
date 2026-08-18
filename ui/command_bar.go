package ui

import (
	"slices"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/noelruault/lazyaws/ui/layout"

	"github.com/noelruault/lazyaws/ui/resources"
)

type commandState struct {
	active bool
	hint   string
}

// staticCommands resolves before resources so resource names cannot shadow verbs.
// Keeping it callable avoids an initialization cycle through the keybinding table.
func staticCommands() map[string]func(gui *Gui, args string) error {
	return map[string]func(gui *Gui, args string) error{
		"filter": (*Gui).cmdFilter,
		"help":   (*Gui).cmdHelp,
		"quit":   (*Gui).cmdQuit,
	}
}

func (gui *Gui) commandPrompt() string {
	return ":"
}

// handleOpenCommandBar is view-bound because a global colon binding would swallow text input.
func (gui *Gui) handleOpenCommandBar() error {
	gui.State.Command.active = true
	gui.Views.Command.ClearTextArea()

	if err := gui.switchFocus(gui.Views.Command); err != nil {
		return err
	}

	return gui.onNewCommandInput("")
}

func (gui *Gui) onNewCommandInput(value string) error {
	gui.State.Command.hint = gui.commandHint(value)
	return gui.renderCommandHint()
}

func (gui *Gui) commandHint(input string) string {
	if verb, _, ok := splitCommand(input); ok {
		return "run " + verb
	}

	if ref, err := gui.Registry.Resolve(input); err == nil {
		if entry, found := gui.Registry.Get(ref.Key()); found {
			if len(ref.Path) > 0 {
				return "→ " + entry.Title + ": " + strings.Join(ref.Path, "/")
			}
			return "→ " + entry.Title
		}
	}

	if suggestions := gui.commandSuggestions(input); len(suggestions) > 0 {
		return strings.Join(suggestions, "  ")
	}

	return "no match"
}

// commandCandidates stays uncapped so completion includes candidates hidden from suggestions.
func (gui *Gui) commandCandidates(input string) []string {
	typed := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(input), resources.Separator))

	out := []string{}
	for _, verb := range sortedCommandVerbs() {
		if strings.HasPrefix(verb, typed) {
			out = append(out, verb)
		}
	}

	return append(out, gui.Registry.Matches(input)...)
}

func (gui *Gui) commandSuggestions(input string) []string {
	candidates := gui.commandCandidates(input)
	if len(candidates) > resources.MaxSuggestions {
		return candidates[:resources.MaxSuggestions]
	}
	return candidates
}

func (gui *Gui) renderCommandHint() error {
	return gui.setViewContent(gui.Views.CommandHint, gui.State.Command.hint)
}

// commitCommand preserves invalid input so typos remain correctable.
func (gui *Gui) commitCommand() error {
	input := gui.Views.Command.TextArea.GetContent()

	if verb, args, ok := splitCommand(input); ok {
		if err := gui.closeCommandBar(); err != nil {
			return err
		}
		return staticCommands()[verb](gui, args)
	}

	ref, err := gui.Registry.Resolve(input)
	if err != nil {
		gui.State.Command.hint = err.Error()
		return gui.renderCommandHint()
	}

	if err := gui.closeCommandBar(); err != nil {
		return err
	}

	if err := gui.Registry.FocusRef(ref); err != nil {
		return gui.createErrorPanel(err.Error())
	}

	return nil
}

func (gui *Gui) completeCommand() error {
	view := gui.Views.Command
	typed := view.TextArea.GetContent()

	// Completion includes hidden candidates so it cannot type an unsafe common prefix.
	completion := resources.CommonPrefix(gui.commandCandidates(typed))
	if completion == "" || completion == strings.TrimPrefix(strings.TrimSpace(typed), resources.Separator) {
		return gui.onNewCommandInput(typed)
	}

	view.ClearTextArea()
	view.TextArea.TypeString(completion)
	view.RenderTextArea()

	return gui.onNewCommandInput(completion)
}

func (gui *Gui) escapeCommandBar() error {
	return gui.closeCommandBar()
}

func (gui *Gui) closeCommandBar() error {
	gui.State.Command.active = false
	gui.State.Command.hint = ""
	gui.Views.Command.ClearTextArea()
	gui.Views.CommandHint.Clear()

	return gui.returnFocus()
}

func splitCommand(input string) (verb, args string, ok bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), resources.Separator))
	verb, args, _ = strings.Cut(trimmed, " ")
	verb = strings.ToLower(verb)

	_, ok = staticCommands()[verb]
	return verb, strings.TrimSpace(args), ok
}

// sortedCommandVerbs keeps the suggestion list from reordering itself between keystrokes, which a map range would.
func sortedCommandVerbs() []string {
	commands := staticCommands()
	verbs := make([]string, 0, len(commands))
	for verb := range commands {
		verbs = append(verbs, verb)
	}
	slices.Sort(verbs)
	return verbs
}

func (gui *Gui) cmdFilter(args string) error {
	if err := gui.handleOpenFilter(); err != nil {
		return err
	}
	if args == "" || !gui.State.Filter.active {
		return nil
	}

	gui.Views.Filter.ClearTextArea()
	gui.Views.Filter.TextArea.TypeString(args)
	gui.Views.Filter.RenderTextArea()

	return gui.onNewFilterNeedle(args)
}

func (gui *Gui) cmdHelp(string) error {
	return gui.handleCreateOptionsMenu(gui.g, gui.g.CurrentView())
}

func (gui *Gui) cmdQuit(string) error {
	return gui.quit(gui.g, gui.g.CurrentView())
}

// commandHintWidth caps suggestions so they cannot squeeze out the input.
func (gui *Gui) commandHintWidth() int {
	width, _ := gui.g.Size()
	return min(runewidth.StringWidth(gui.State.Command.hint)+runewidth.StringWidth(infoSectionPadding), width/2)
}

func (gui *Gui) commandBarBoxes() []*layout.Box {
	return []*layout.Box{
		{Window: "commandPrefix", Size: runewidth.StringWidth(gui.commandPrompt())},
		{Window: "command", Weight: 1},
		{Window: "commandHint", Size: gui.commandHintWidth()},
	}
}
