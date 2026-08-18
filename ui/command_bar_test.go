package ui

import (
	"strings"
	"testing"

	"github.com/jesseduffield/gocui"
	"github.com/noelruault/lazyaws/ui/layout"

	"github.com/noelruault/lazyaws/ui/resources"
)

// View access goes through the gocui loop to avoid racing its renderer.

func openBar(t *testing.T, g *gocui.Gui, gui *Gui, text string) {
	t.Helper()

	run(t, g, gui.handleOpenCommandBar)
	typeCommand(t, g, gui, text)
}

func typeCommand(t *testing.T, g *gocui.Gui, gui *Gui, text string) {
	t.Helper()

	run(t, g, func() error {
		gui.Views.Command.ClearTextArea()
		gui.Views.Command.TextArea.TypeString(text)
		gui.Views.Command.RenderTextArea()
		return gui.onNewCommandInput(text)
	})
}

func focusedView(g *gocui.Gui, gui *Gui) string {
	return ask(g, gui.currentViewName)
}

func commandHintText(g *gocui.Gui, gui *Gui) string {
	return ask(g, func() string { return gui.State.Command.hint })
}

func commandInput(g *gocui.Gui, gui *Gui) string {
	return ask(g, func() string { return gui.Views.Command.TextArea.GetContent() })
}

func TestColonOpensTheCommandBar(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, gui.handleOpenCommandBar)

	if got := focusedView(g, gui); got != "command" {
		t.Errorf("focused %q, want the command view", got)
	}
	if !ask(g, func() bool { return gui.State.Command.active }) {
		t.Error("the bar is focused but not marked active, so the bottom line will not show it")
	}
}

// The command chord must not steal literal input from text fields.
func TestColonIsBoundWhereTypingIsNot(t *testing.T) {
	gui, g := newHeadlessGui(t)

	bound := map[string]bool{}
	for _, binding := range ask(g, gui.GetInitialKeybindings) {
		if key, ok := binding.Key.(rune); ok && key == ':' {
			bound[binding.ViewName] = true
		}
	}

	if bound[""] {
		t.Error(`":" is bound globally, which would swallow a colon typed into any input`)
	}
	for _, name := range []string{"qInput", "filter", "command", "confirmation", "settings"} {
		if bound[name] {
			t.Errorf(`":" is bound on %q, which is a view you type into`, name)
		}
	}
	for _, name := range append(sidePanelViewNames(gui.allSidePanels()), "main") {
		if !bound[name] {
			t.Errorf(`":" is not bound on %q`, name)
		}
	}
}

func TestCommitCommandNavigates(t *testing.T) {
	gui, g := newHeadlessGui(t)

	openBar(t, g, gui, "ecr")
	run(t, g, gui.commitCommand)

	if got := focusedView(g, gui); got != "ecr" {
		t.Errorf("focused %q, want ecr", got)
	}
	if ask(g, func() bool { return gui.State.Command.active }) {
		t.Error("the bar stayed open after a successful jump")
	}
}

// Resolution errors must preserve the user's input for correction.
func TestCommitCommandKeepsTheBarOpenOnATypo(t *testing.T) {
	gui, g := newHeadlessGui(t)

	openBar(t, g, gui, "zzzzzz")
	run(t, g, gui.commitCommand)

	if got := focusedView(g, gui); got != "command" {
		t.Errorf("focus moved to %q on an unresolvable input", got)
	}
	if !ask(g, func() bool { return gui.State.Command.active }) {
		t.Error("the bar closed on an unresolvable input")
	}
	if got := commandInput(g, gui); got != "zzzzzz" {
		t.Errorf("what was typed became %q; it should still be editable", got)
	}
	if commandHintText(g, gui) == "" {
		t.Error("no reason shown for the input being rejected")
	}
}

func TestEscapeReturnsFocusToWhereItCameFrom(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, func() error {
		view, err := g.View("eks")
		if err != nil {
			return err
		}
		return gui.switchFocus(view)
	})

	openBar(t, g, gui, "s3")
	run(t, g, gui.escapeCommandBar)

	if got := focusedView(g, gui); got != "eks" {
		t.Errorf("esc landed on %q, want eks", got)
	}
	if ask(g, func() bool { return gui.State.Command.active }) {
		t.Error("esc left the bar active")
	}
	if got := commandInput(g, gui); got != "" {
		t.Errorf("esc left %q in the bar", got)
	}
}

// Preview resolution must not navigate before Enter.
func TestHintPreviewsWithoutNavigating(t *testing.T) {
	gui, g := newHeadlessGui(t)

	openBar(t, g, gui, "s3")
	if !strings.Contains(commandHintText(g, gui), "S3 Buckets") {
		t.Errorf("hint for s3 = %q, want it to name the destination", commandHintText(g, gui))
	}
	if got := focusedView(g, gui); got != "command" {
		t.Fatalf("typing moved focus to %q", got)
	}

	typeCommand(t, g, gui, "ec")
	hint := commandHintText(g, gui)
	if !strings.Contains(hint, "ec2") || !strings.Contains(hint, "ecr") {
		t.Errorf("hint for an ambiguous prefix = %q, want the candidates listed", hint)
	}

	typeCommand(t, g, gui, "zzzzzz")
	if got := commandHintText(g, gui); got != "no match" {
		t.Errorf("hint for nonsense = %q, want %q", got, "no match")
	}
}

func TestHintShowsTheSelectorPath(t *testing.T) {
	gui, g := newHeadlessGui(t)

	openBar(t, g, gui, "ecs:my-cluster:my-service")

	if got := commandHintText(g, gui); !strings.Contains(got, "my-cluster/my-service") {
		t.Errorf("hint = %q, want the selector previewed", got)
	}
}

func TestTabCompletesToTheSharedStem(t *testing.T) {
	gui, g := newHeadlessGui(t)

	openBar(t, g, gui, "e")
	run(t, g, gui.completeCommand)
	if got := commandInput(g, gui); got != "e" {
		t.Errorf("tab on an unshared prefix expanded to %q", got)
	}

	typeCommand(t, g, gui, "sec")
	run(t, g, gui.completeCommand)

	got := commandInput(g, gui)
	if got == "sec" || !strings.HasPrefix(got, "sec") {
		t.Errorf("tab expanded %q to %q, want a longer completion", "sec", got)
	}
	if _, err := gui.Registry.Resolve(got); err != nil {
		t.Errorf("tab produced %q, which does not resolve: %v", got, err)
	}
}

func TestTabCompletesStaticCommands(t *testing.T) {
	gui, g := newHeadlessGui(t)

	openBar(t, g, gui, "qui")
	run(t, g, gui.completeCommand)

	if got := commandInput(g, gui); got != "quit" {
		t.Errorf("tab expanded %q to %q, want %q", "qui", got, "quit")
	}
}

// Completion must include candidates hidden by the suggestion display cap.
func TestTabNeverCompletesPastACandidate(t *testing.T) {
	gui, g := newHeadlessGui(t)

	for _, typed := range []string{"", "a", "e", "s", "c", "p"} {
		candidates := ask(g, func() []string { return gui.commandCandidates(typed) })
		completion := resources.CommonPrefix(candidates)

		for _, candidate := range candidates {
			if !strings.HasPrefix(candidate, completion) {
				t.Errorf("tab would complete %q to %q, which the candidate %q does not start with", typed, completion, candidate)
			}
		}
	}
}

// Registered resources must not shadow static commands.
func TestStaticCommandsWinOverResources(t *testing.T) {
	gui, g := newHeadlessGui(t)

	for verb := range staticCommands() {
		if _, _, ok := splitCommand(":" + verb); !ok {
			t.Errorf("%q is not recognised as a command", verb)
		}
		if _, _, ok := splitCommand(":" + verb + " some args"); !ok {
			t.Errorf("%q with arguments is not recognised as a command", verb)
		}
	}

	if _, args, _ := splitCommand(":filter  prod  "); args != "prod" {
		t.Errorf("arguments parsed as %q, want %q", args, "prod")
	}
	if _, _, ok := splitCommand(":ec2"); ok {
		t.Error("a resource was taken for a command")
	}

	openBar(t, g, gui, "help")
	if got := commandHintText(g, gui); !strings.Contains(got, "help") {
		t.Errorf("hint for a command = %q", got)
	}
}

func TestFilterCommandSeedsTheFilter(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, func() error {
		view, err := g.View("ecr")
		if err != nil {
			return err
		}
		return gui.switchFocus(view)
	})

	openBar(t, g, gui, "filter prod")
	run(t, g, gui.commitCommand)

	if !ask(g, func() bool { return gui.State.Filter.active }) {
		t.Fatal(":filter did not open the filter")
	}
	if got := ask(g, func() string { return gui.State.Filter.needle }); got != "prod" {
		t.Errorf("filter needle = %q, want %q", got, "prod")
	}
	if got := ask(g, func() string { return gui.Views.Filter.TextArea.GetContent() }); got != "prod" {
		t.Errorf("filter input shows %q, want %q", got, "prod")
	}
	if got := ask(g, func() string {
		if gui.State.Filter.panel == nil {
			return ""
		}
		return gui.State.Filter.panel.GetView().Name()
	}); got != "ecr" {
		t.Errorf(":filter targeted %q, want the panel the bar was opened from", got)
	}
}

func TestCommandBarTakesOverTheBottomLine(t *testing.T) {
	gui, g := newHeadlessGui(t)

	openBar(t, g, gui, "s3")

	windows := ask(g, func() map[string]layout.Dimensions { return gui.getWindowDimensions("", "") })
	for _, name := range []string{"commandPrefix", "command", "commandHint"} {
		dims, ok := windows[name]
		if !ok {
			t.Errorf("%q is not in the layout while the bar is open", name)
			continue
		}
		// boxlayout's bounds are inclusive, so a one-column box has X1 == X0.
		if dims.X1 < dims.X0 {
			t.Errorf("%q was given no width: %+v", name, dims)
		}
	}
	if _, ok := windows["options"]; ok {
		t.Error("the options line is still laid out under the command bar")
	}

	run(t, g, gui.escapeCommandBar)

	closed := ask(g, func() map[string]layout.Dimensions { return gui.getWindowDimensions("", "") })
	if _, ok := closed["command"]; ok {
		t.Error("the command bar kept the bottom line after closing")
	}
}

func TestCommandBarEditorPreviewsAsYouType(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, gui.handleOpenCommandBar)
	run(t, g, func() error {
		for _, r := range "eks" {
			gui.Views.Command.Editor.Edit(gui.Views.Command, 0, r, gocui.ModNone)
		}
		return nil
	})

	if got := commandInput(g, gui); got != "eks" {
		t.Fatalf("typed buffer = %q, want %q", got, "eks")
	}
	if got := commandHintText(g, gui); !strings.Contains(got, "EKS") {
		t.Errorf("hint after typing = %q, want it to name the EKS destination", got)
	}
}
