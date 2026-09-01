package ui

import (
	"github.com/jesseduffield/gocui"
)

type Binding struct {
	ViewName    string
	Handler     func(*gocui.Gui, *gocui.View) error
	Key         interface{} // rune | gocui.Key
	Modifier    gocui.Modifier
	Description string

	// Name is empty for literals; named bindings are rebindable and conflict-checked.
	Name KeyName
}

func (b *Binding) GetKey() string {
	switch key := b.Key.(type) {
	case rune:
		if key == ' ' {
			return "space"
		}
		return string(key)
	case gocui.Key:
		switch key {
		case gocui.KeyEsc:
			return "esc"
		case gocui.KeyEnter:
			return "enter"
		case gocui.KeyTab:
			return "tab"
		case gocui.KeySpace:
			return "space"
		case gocui.KeyArrowRight:
			return "►"
		case gocui.KeyArrowLeft:
			return "◄"
		case gocui.KeyArrowUp:
			return "▲"
		case gocui.KeyArrowDown:
			return "▼"
		case gocui.KeyPgup:
			return "PgUp"
		case gocui.KeyPgdn:
			return "PgDn"
		// A key with no label here is dropped from the menu by getBindings, so anything bound has to be nameable.
		case gocui.KeyBacktab:
			return "shift+tab"
		case gocui.KeyHome:
			return "Home"
		case gocui.KeyEnd:
			return "End"
		}

		if key >= gocui.KeyCtrlA && key <= gocui.KeyCtrlZ {
			return "ctrl+" + string(rune('a'+key-gocui.KeyCtrlA))
		}
	}

	return ""
}

func (gui *Gui) quit(g *gocui.Gui, v *gocui.View) error {
	if gui.Config.User.ConfirmOnQuit {
		return gui.createConfirmationPanel("", "Are you sure you want to quit?", func(g *gocui.Gui, v *gocui.View) error {
			return gocui.ErrQuit
		}, nil)
	}
	return gocui.ErrQuit
}

func (gui *Gui) escape() error {
	if gui.State.Filter.active {
		return gui.clearFilter()
	}
	return nil
}

func (gui *Gui) handleRefreshAll() error {
	gui.throttledRefresh.Trigger()
	return nil
}

func (gui *Gui) refreshFocusedPanel() error {
	panel, ok := gui.currentSidePanel()
	if !ok {
		return nil
	}
	t, ok := gui.panelThrottles[panel.GetView().Name()]
	if !ok {
		return nil
	}
	t.Trigger()
	return nil
}

func wrappedHandler(f func() error) func(*gocui.Gui, *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		return f()
	}
}

// key resolves through the keymap so rebinding cannot stale help text.
func (gui *Gui) key(viewName string, name KeyName, handler func(*gocui.Gui, *gocui.View) error) *Binding {
	chord := gui.Keys.Get(name)

	return &Binding{
		ViewName:    viewName,
		Name:        name,
		Key:         chord.Key,
		Modifier:    chord.Modifier,
		Handler:     handler,
		Description: chord.Description,
	}
}

// describedKey is key() for a chord whose job depends on the view it is bound in.
// The keymap's own description has to stay general enough to cover every one of them, so the menu takes this instead: it is read by someone looking at one view, and "go to the main panel" is what the key does there.
func (gui *Gui) describedKey(viewName string, name KeyName, handler func(*gocui.Gui, *gocui.View) error, description string) *Binding {
	binding := gui.key(viewName, name, handler)
	binding.Description = description

	return binding
}

func (gui *Gui) GetInitialKeybindings() []*Binding {
	bindings := []*Binding{
		{ViewName: "", Key: gocui.KeyEsc, Handler: wrappedHandler(gui.escape), Description: "Close what is open, or step back one level"},
		gui.key("", KeyQuit, gui.quit),
		{ViewName: "", Key: gocui.KeyCtrlC, Handler: gui.quit, Description: "Quit"},
		gui.key("", KeyScrollMainPageUp, wrappedHandler(gui.scrollUpMain)),
		gui.key("", KeyScrollMainPageDown, wrappedHandler(gui.scrollDownMain)),
		gui.key("", KeyScrollMainUp, wrappedHandler(gui.scrollUpMain)),
		gui.key("", KeyScrollMainDown, wrappedHandler(gui.scrollDownMain)),
		{ViewName: "", Key: gocui.KeyEnd, Handler: gui.autoScrollMain, Description: "Follow the bottom of the main panel"},
		{ViewName: "", Key: gocui.KeyHome, Handler: gui.jumpToTopMain, Description: "Jump to the top of the main panel"},
		gui.key("", KeyOptionsMenu, gui.handleCreateOptionsMenu),
		gui.key("", KeyHelp, gui.handleCreateOptionsMenu),
		gui.key("", KeyRefreshAll, wrappedHandler(gui.handleRefreshAll)),
		gui.key("", KeyRedraw, wrappedHandler(gui.handleRedraw)),
		gui.key("", KeyRefreshPanel, wrappedHandler(gui.refreshFocusedPanel)),
		gui.key("", KeyScreenModeNext, wrappedHandler(gui.nextScreenMode)),
		gui.key("", KeyScreenModePrev, wrappedHandler(gui.prevScreenMode)),
		gui.key("", KeySettings, wrappedHandler(gui.handleToggleSettings)),
		gui.key("", KeyAmazonQ, wrappedHandler(gui.handleToggleQ)),

		{ViewName: "main", Key: gocui.KeyEsc, Handler: gui.handleQMainEscape, Description: "back to the panel column"},
		{ViewName: "main", Key: gocui.KeyTab, Handler: wrappedHandler(gui.handleMainTabNext), Description: "next detail tab"},
		{ViewName: "main", Key: gocui.KeyBacktab, Handler: wrappedHandler(gui.handleMainTabPrev), Description: "previous detail tab"},
		{ViewName: "main", Key: gocui.KeyArrowLeft, Handler: gui.scrollLeftMain, Description: "scroll left"},
		{ViewName: "main", Key: gocui.KeyArrowRight, Handler: gui.scrollRightMain, Description: "scroll right"},
		gui.key("main", KeyNavLeft, gui.scrollLeftMain),
		gui.describedKey("main", KeyNavRight, gui.scrollRightMain, "scroll right"),
		gui.key("main", KeyPrevTab, wrappedHandler(gui.handleMainPrevTab)),
		gui.key("main", KeyNextTab, wrappedHandler(gui.handleMainNextTab)),
		{ViewName: "main", Key: gocui.KeyEnter, Handler: wrappedHandler(gui.handleMainEnter), Description: "select"},
		gui.key("main", KeyActions, wrappedHandler(gui.handleMainAction)),

		{ViewName: "filter", Key: gocui.KeyEnter, Handler: wrappedHandler(gui.commitFilter)},
		{ViewName: "filter", Key: gocui.KeyEsc, Handler: wrappedHandler(gui.escapeFilterPrompt)},

		{ViewName: "command", Key: gocui.KeyEnter, Handler: wrappedHandler(gui.commitCommand), Description: "go"},
		{ViewName: "command", Key: gocui.KeyEsc, Handler: wrappedHandler(gui.escapeCommandBar), Description: "cancel"},
		{ViewName: "command", Key: gocui.KeyTab, Handler: wrappedHandler(gui.completeCommand), Description: "complete"},

		{ViewName: "settings", Key: gocui.KeyEsc, Handler: wrappedHandler(gui.handleExitSettings), Description: "back to dashboard"},
		{ViewName: "settings", Key: ' ', Handler: wrappedHandler(gui.handleSettingsToggle), Description: "toggle"},
		{ViewName: "settings", Key: gocui.KeyEnter, Handler: wrappedHandler(gui.handleSettingsToggle), Description: "toggle"},
		gui.key("settings", KeySettingsEditFile, wrappedHandler(gui.handleSettingsEditFile)),

		{ViewName: "qInput", Key: gocui.KeyEnter, Handler: wrappedHandler(gui.handleQSubmit), Description: "ask"},
		{ViewName: "qInput", Key: gocui.KeyEsc, Handler: wrappedHandler(gui.handleExitQ), Description: "back to dashboard"},
		{ViewName: "qInput", Key: gocui.KeyTab, Handler: wrappedHandler(gui.handleQFocusNext), Description: "next pane"},
		{ViewName: "qChats", Key: gocui.KeyEsc, Handler: wrappedHandler(gui.handleExitQ), Description: "back to dashboard"},
		{ViewName: "qChats", Key: gocui.KeyTab, Handler: wrappedHandler(gui.handleQFocusNext), Description: "next pane"},
		{ViewName: "qChats", Key: gocui.KeyEnter, Handler: wrappedHandler(gui.handleQFocusInput), Description: "focus input"},
		gui.key("qInput", KeyChatToggleFolds, wrappedHandler(gui.handleQToggleFolds)),
		gui.key("qChats", KeyChatToggleFolds, wrappedHandler(gui.handleQToggleFolds)),
		// Character shortcuts would be typed into qInput, so model selection uses a control key there.
		gui.key("qInput", KeyChatPickModel, wrappedHandler(gui.handleQPickModel)),
		gui.key("qChats", KeyChatPickModel, wrappedHandler(gui.handleQPickModel)),
		gui.key("qChats", KeyActions, wrappedHandler(gui.handleQPickModel)),
		// qInput owns vertical keys because its single-line editor would otherwise swallow conversation scrolling.
		{ViewName: "qInput", Key: gocui.KeyArrowUp, Handler: wrappedHandler(gui.scrollUpMain), Description: "scroll conversation"},
		{ViewName: "qInput", Key: gocui.KeyArrowDown, Handler: wrappedHandler(gui.scrollDownMain), Description: "scroll conversation"},
		gui.key("qInput", KeyScrollMainPageUp, wrappedHandler(gui.scrollUpMain)),
		gui.key("qInput", KeyScrollMainPageDown, wrappedHandler(gui.scrollDownMain)),
		gui.key("qInput", KeyChatNewConversation, wrappedHandler(gui.handleQNewChat)),
		gui.key("qChats", KeyChatNewConversation, wrappedHandler(gui.handleQNewChat)),

		{ViewName: "menu", Key: gocui.KeyEsc, Handler: wrappedHandler(gui.handleMenuClose)},
		{ViewName: "menu", Key: 'q', Handler: wrappedHandler(gui.handleMenuClose)},
		{ViewName: "menu", Key: ' ', Handler: wrappedHandler(gui.handleMenuPress)},
		{ViewName: "menu", Key: gocui.KeyEnter, Handler: wrappedHandler(gui.handleMenuPress)},
		{ViewName: "menu", Key: 'y', Handler: wrappedHandler(gui.handleMenuPress)},
	}

	bindings = append(bindings,
		&Binding{Key: '1', Handler: gui.handleGoTo(gui.Views.Profile), Description: "focus profile panel"},
		&Binding{Key: '2', Handler: gui.handleGoTo(gui.Views.ECS), Description: "focus ecs panel"},
		&Binding{Key: '3', Handler: gui.handleGoTo(gui.Views.EC2), Description: "focus ec2 panel"},
		&Binding{Key: '4', Handler: gui.handleGoTo(gui.Views.S3), Description: "focus s3 panel"},
		&Binding{Key: '5', Handler: gui.handleGoTo(gui.Views.EKS), Description: "focus eks panel"},
		&Binding{Key: '6', Handler: gui.handleGoTo(gui.Views.ECR), Description: "focus ecr panel"},
		&Binding{Key: '7', Handler: gui.handleGoTo(gui.Views.Secrets), Description: "focus secrets panel"},
		&Binding{Key: '8', Handler: gui.handleGoTo(gui.Views.VPC), Description: "focus vpc panel"},
	)

	// Left and right walk the panel column, the same as Tab and Shift+Tab: eight lists stacked in one column are what the four keys are for, and Enter is the one that leaves it for the pane beside them.
	for _, panel := range gui.allSidePanels() {
		name := panel.GetView().Name()
		bindings = append(bindings,
			&Binding{ViewName: name, Key: gocui.KeyArrowLeft, Handler: gui.previousView, Description: "previous panel"},
			&Binding{ViewName: name, Key: gocui.KeyArrowRight, Handler: gui.nextView, Description: "next panel"},
			gui.describedKey(name, KeyNavLeft, gui.previousView, "previous panel"),
			gui.describedKey(name, KeyNavRight, gui.nextView, "next panel"),
			&Binding{ViewName: name, Key: gocui.KeyTab, Handler: gui.nextView, Description: "next panel"},
			&Binding{ViewName: name, Key: gocui.KeyBacktab, Handler: gui.previousView, Description: "previous panel"},
		)
	}

	// The arrows carry the same description as their vim key, so the menu lists the pair on one row rather than saying the same thing twice.
	setUpDownClickBindings := func(viewName string, onUp, onDown, onClick func() error) {
		bindings = append(bindings,
			gui.key(viewName, KeyNavUp, wrappedHandler(onUp)),
			&Binding{ViewName: viewName, Key: gocui.KeyArrowUp, Handler: wrappedHandler(onUp), Description: gui.Keys.Get(KeyNavUp).Description},
			&Binding{ViewName: viewName, Key: gocui.MouseWheelUp, Handler: wrappedHandler(onUp)},
			gui.key(viewName, KeyNavDown, wrappedHandler(onDown)),
			&Binding{ViewName: viewName, Key: gocui.KeyArrowDown, Handler: wrappedHandler(onDown), Description: gui.Keys.Get(KeyNavDown).Description},
			&Binding{ViewName: viewName, Key: gocui.MouseWheelDown, Handler: wrappedHandler(onDown)},
			&Binding{ViewName: viewName, Key: gocui.MouseLeft, Handler: wrappedHandler(onClick)},
		)
	}

	for _, panel := range gui.allListPanels() {
		setUpDownClickBindings(panel.GetView().Name(), panel.HandlePrevLine, panel.HandleNextLine, panel.HandleClick)
	}
	setUpDownClickBindings("main", gui.handleMainUp, gui.handleMainDown, gui.handleQConversationClick)
	setUpDownClickBindings("qChats", gui.handleQPrevChat, gui.handleQNextChat, gui.handleQChatsClick)
	setUpDownClickBindings("settings", gui.handleSettingsPrevLine, gui.handleSettingsNextLine, gui.handleSettingsClick)

	// Overrides precede generic panel bindings because dispatch stops at the first view match.
	bindings = append(bindings,
		&Binding{ViewName: "profile", Key: gocui.KeyEnter, Handler: gui.handleProfileSwitch, Description: "switch profile"},
		&Binding{ViewName: "ecs", Key: gocui.KeyEnter, Handler: gui.handleECSDrillDown, Description: "drill down"},
		&Binding{ViewName: "ecs", Key: gocui.KeyEsc, Handler: gui.handleECSEscape, Description: "drill up"},
		gui.key("ecs", KeyECSExec, gui.handleECSExec),
		gui.key("ec2", KeyEC2Connect, gui.handleEC2Connect),
		gui.key("secrets", KeySecretsReveal, gui.handleSecretsToggleReveal),
		gui.key("secrets", KeySecretsToggleDeleted, gui.handleSecretsToggleDeleted),
	)

	for _, panel := range gui.allSidePanels() {
		name := panel.GetView().Name()
		bindings = append(bindings,
			// The same words as the right arrow's, so the menu shows one row for the one thing they both do.
			&Binding{ViewName: name, Key: gocui.KeyEnter, Handler: gui.handleEnterMain, Description: "go to the main panel"},
			gui.key(name, KeyPrevTab, wrappedHandler(panel.HandlePrevMainTab)),
			gui.key(name, KeyNextTab, wrappedHandler(panel.HandleNextMainTab)),
		)
	}

	// One registry-backed action handler keeps service additions out of the binding table.
	for _, name := range sidePanelViewNames(gui.allSidePanels()) {
		bindings = append(bindings, gui.key(name, KeyActions, wrappedHandler(gui.handleActionsMenu)))
	}

	// A global colon binding would swallow input in chat, filters, and prompts, so only non-input views register it.
	// The copy key is scoped the same way for a second reason: the menu and confirmation popups bind y as "yes", and a global binding there would be shadowed on exactly the views that already answer to it.
	for _, name := range resourceViewNames(gui.allSidePanels()) {
		bindings = append(bindings,
			gui.key(name, KeyCommandBar, wrappedHandler(gui.handleOpenCommandBar)),
			gui.key(name, KeyCopyID, wrappedHandler(gui.handleCopySelected)),
		)
	}

	for _, panel := range gui.allListPanels() {
		if !panel.IsFilterDisabled() {
			bindings = append(bindings, gui.key(panel.GetView().Name(), KeyFilter, wrappedHandler(gui.handleOpenFilter)))
		}
	}

	return bindings
}

func (gui *Gui) keybindings(g *gocui.Gui) error {
	bindings := gui.GetInitialKeybindings()

	// Collision checks use registered bindings because only they know each key's view scope.
	gui.startupProblems = append(gui.startupProblems, checkKeyConflicts(bindings)...)

	for _, binding := range bindings {
		if err := g.SetKeybinding(binding.ViewName, binding.Key, binding.Modifier, binding.Handler); err != nil {
			return err
		}
	}

	return g.SetTabClickBinding("main", gui.onMainTabClick)
}
