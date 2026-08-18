package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/ui/resources"
	"github.com/noelruault/lazyaws/ui/types"
)

func (gui *Gui) handleActionsMenu() error {
	entry, ok := gui.focusedEntry()
	if !ok || entry.Actions == nil {
		return nil
	}

	return gui.openActionsMenu(entry.Title+" actions", entry.Actions())
}

func (gui *Gui) openActionsMenu(title string, actions []resources.Action) error {
	if len(actions) == 0 {
		return nil
	}

	return gui.Menu(CreateMenuOptions{Title: title, Items: gui.actionMenuItems(actions)})
}

func (gui *Gui) focusedEntry() (*resources.Entry, bool) {
	key, ok := panelRefs()[gui.currentViewName()]
	if !ok {
		return nil, false
	}

	return gui.Registry.Get(key)
}

// actionMenuItems preserves Mutates because menus enforce read-only visibility separately.
func (gui *Gui) actionMenuItems(actions []resources.Action) []*types.MenuItem {
	items := make([]*types.MenuItem, 0, len(actions))
	for _, action := range actions {
		items = append(items, &types.MenuItem{
			Label:   action.Name,
			Mutates: action.Mutates,
			OnPress: func() error { return gui.runAction(action) },
		})
	}

	return items
}

// runAction is the enforcement boundary for read-only and confirmation gates.
func (gui *Gui) runAction(action resources.Action) error {
	// Tokens are built from live AWS data, so an empty token is a runtime possibility and would make the DANGER prompt satisfiable by enter alone.
	if err := action.Valid(); err != nil {
		return gui.createErrorPanel(action.Name + ": " + err.Error())
	}

	if action.Mutates && gui.readOnly() {
		return gui.refuseReadOnly(action.Name)
	}

	if action.Prompt != "" {
		return gui.createPromptPanel(action.Prompt, gui.onActionInput(action))
	}

	return gui.confirmAction(action, "")
}

func (gui *Gui) onActionInput(action resources.Action) func(*gocui.Gui, *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		return gui.confirmAction(action, gui.trimmedContent(v))
	}
}

func (gui *Gui) confirmAction(action resources.Action, input string) error {
	switch action.Confirm {
	case resources.ConfirmDangerous:
		return gui.createPromptPanelWithFrame(fmt.Sprintf("DANGER: type %q to confirm", action.Token), gocui.ColorRed, gui.onDangerousToken(action, input))
	case resources.ConfirmSimple:
		return gui.createConfirmationPanel(action.Name, actionConfirmation(action), gui.onActionConfirmed(action, input), nil)
	default:
		return gui.execAction(action, input)
	}
}

// onDangerousToken re-prompts after typos so users are not trained to paste tokens blindly.
func (gui *Gui) onDangerousToken(action resources.Action, input string) func(*gocui.Gui, *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		if gui.trimmedContent(v) != action.Token {
			return gui.confirmAction(action, input)
		}
		return gui.execAction(action, input)
	}
}

func (gui *Gui) onActionConfirmed(action resources.Action, input string) func(*gocui.Gui, *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		return gui.execAction(action, input)
	}
}

func (gui *Gui) execAction(action resources.Action, input string) error {
	return gui.WithWaitingStatus(strings.ToLower(action.Name), func() error {
		ctx, cancel := context.WithTimeout(context.Background(), action.Deadline())
		defer cancel()

		return action.Run(ctx, input)
	})
}

func actionConfirmation(action resources.Action) string {
	question := action.Confirmation
	if question == "" {
		question = action.Name + "?"
	}

	return color.New(color.FgRed).SprintFunc()(question)
}
