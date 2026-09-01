package ui

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/config"
	"github.com/noelruault/lazyaws/ui/presentation"
	"github.com/noelruault/lazyaws/ui/utils"
)

func (gui *Gui) readOnly() bool {
	return gui.Config != nil && gui.Config.User.ReadOnly
}

func (gui *Gui) refuseReadOnly(what string) error {
	return gui.createConfirmationPanel("Read-only mode", what+" changes things, so read-only mode won't do it.\n\nTurn read-only mode off in Settings (press o) if you meant to.", nil, nil)
}

type settingsState struct {
	active   bool
	selected int

	// mu guards the model list, which is fetched from Bedrock on a background goroutine and read by the render.
	mu        sync.Mutex
	models    []string
	modelsErr string
}

type setting struct {
	name string
	help string
	path []string

	get func(*config.UserConfig) bool
	set func(*config.UserConfig, bool)

	choices   func() []string
	getChoice func(*config.UserConfig) string
	setChoice func(*config.UserConfig, string)
	emptyHint string

	// seconds is the interval kind: a ladder of values the row cycles through, because the Settings screen has no text entry and an interval is not a string choice.
	// It needs its own kind rather than reusing choices: the YAML tag differs, and writing an int key as a quoted string leaves a file the next load cannot parse.
	seconds []int
	getInt  func(*config.UserConfig) int
	setInt  func(*config.UserConfig, int)
}

func (s setting) isChoice() bool { return s.choices != nil }

func (s setting) isSeconds() bool { return s.seconds != nil }

func (gui *Gui) settings() []setting {
	return []setting{
		{
			name: "Chat",
			help: "ask an AI about this account, using your AWS credentials",
			path: []string{"chat", "enabled"},
			get:  func(user *config.UserConfig) bool { return user.Chat.Enabled },
			set:  func(user *config.UserConfig, value bool) { user.Chat.Enabled = value },
		},
		{
			name:      "Chat backend",
			help:      "bedrock: a plain AWS call; kiro: the Kiro CLI",
			path:      []string{"chat", "provider"},
			choices:   func() []string { return []string{config.ProviderBedrock, config.ProviderKiro} },
			getChoice: func(user *config.UserConfig) string { return user.Chat.Provider },
			setChoice: func(user *config.UserConfig, value string) { user.Chat.Provider = value },
		},
		{
			name:      "Chat model",
			help:      "models this account can call, from Bedrock",
			path:      []string{"chat", "model"},
			choices:   gui.chatModelChoices,
			getChoice: func(user *config.UserConfig) string { return user.Chat.Model },
			setChoice: func(user *config.UserConfig, value string) { user.Chat.Model = value },
			emptyHint: "(no models listed yet)",
		},
		{
			name: "Read-only mode",
			help: "hide every action that changes AWS state",
			path: []string{"readOnly"},
			get:  func(user *config.UserConfig) bool { return user.ReadOnly },
			set:  func(user *config.UserConfig, value bool) { user.ReadOnly = value },
		},
		{
			// The help says "restart" because gocui registers every binding once at startup: the file is written now, the keys move next run.
			name:      "Keybinding preset",
			help:      "navigation layout, applied on restart",
			path:      []string{"keybindingPreset"},
			choices:   PresetNames,
			getChoice: func(user *config.UserConfig) string { return user.KeybindingPreset },
			setChoice: func(user *config.UserConfig, value string) { user.KeybindingPreset = value },
		},
		{
			name: "Dim behind popups",
			help: "fade the dashboard while a popup is open",
			path: []string{"gui", "dimBehindPopups"},
			get:  func(user *config.UserConfig) bool { return user.Gui.DimBehindPopups },
			set:  func(user *config.UserConfig, value bool) { user.Gui.DimBehindPopups = value },
		},
		{
			name:    "Overview refresh",
			help:    "how often the open Overview tab redraws",
			path:    []string{"refresh", "overviewSeconds"},
			seconds: []int{0, 1, 2, 5, 10},
			getInt:  func(user *config.UserConfig) int { return user.Refresh.OverviewSeconds },
			setInt:  func(user *config.UserConfig, value int) { user.Refresh.OverviewSeconds = value },
		},
		{
			name:    "Panel refresh",
			help:    "how often the focused panel's list reloads",
			path:    []string{"refresh", "panelSeconds"},
			seconds: []int{0, 1, 2, 5, 10},
			getInt:  func(user *config.UserConfig) int { return user.Refresh.PanelSeconds },
			setInt:  func(user *config.UserConfig, value int) { user.Refresh.PanelSeconds = value },
		},
		{
			// The ladder starts at the floor rather than at 1: CloudWatch bills per metric per request, so this is the one interval where a shorter setting has a price rather than a cost.
			name:    "Metrics refresh",
			help:    "how often CloudWatch metrics refetch (billed per metric)",
			path:    []string{"refresh", "metricsSeconds"},
			seconds: []int{0, config.MetricsFloorSeconds, 30, 60, 300},
			getInt:  func(user *config.UserConfig) int { return user.Refresh.MetricsSeconds },
			setInt:  func(user *config.UserConfig, value int) { user.Refresh.MetricsSeconds = value },
		},
	}
}

// chatModelChoices keeps the configured model selectable when discovery omits it.
func (gui *Gui) chatModelChoices() []string {
	gui.State.Settings.mu.Lock()
	defer gui.State.Settings.mu.Unlock()

	choices := make([]string, 0, len(gui.State.Settings.models)+1)
	choices = append(choices, gui.State.Settings.models...)

	current := gui.Config.User.Chat.Model
	if current != "" && !slices.Contains(choices, current) {
		choices = append([]string{current}, choices...)
	}

	return choices
}

// loadChatModels keeps discovery failures inline so Settings remains usable.
func (gui *Gui) loadChatModels() {
	if gui.Client == nil {
		return
	}

	client := gui.Client
	gen := gui.Gen

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		models, err := client.ListChatModels(ctx)
		if gen != gui.Gen {
			return
		}

		gui.State.Settings.mu.Lock()
		if err != nil {
			gui.State.Settings.modelsErr = err.Error()
		} else {
			gui.State.Settings.modelsErr = ""
			gui.State.Settings.models = make([]string, 0, len(models))
			for _, model := range models {
				gui.State.Settings.models = append(gui.State.Settings.models, model.ID)
			}
		}
		gui.State.Settings.mu.Unlock()

		gui.renderSettings()
	}()
}

func (gui *Gui) handleToggleSettings() error {
	if gui.State.Settings.active {
		return nil
	}

	// Chat and Settings share the layout slot, so entering one must dismiss the other and its popups.
	gui.dismissPopups()
	gui.leaveQScreen()

	gui.State.Settings.active = true
	gui.renderSettings()
	gui.loadChatModels()

	return gui.switchFocus(gui.Views.Settings)
}

func (gui *Gui) handleExitSettings() error {
	gui.dismissPopups()
	gui.State.Settings.active = false

	// Whatever was toggled may have changed what the dashboard should show (read-only mode hides actions), so its content is re-rendered rather than reused.
	gui.State.Panels.Main.ObjectKey = ""

	view, err := gui.g.View(gui.currentSideViewName())
	if err != nil {
		return err
	}

	return gui.switchFocus(view)
}

func (gui *Gui) handleSettingsPrevLine() error {
	return gui.selectSetting(gui.State.Settings.selected - 1)
}

func (gui *Gui) handleSettingsNextLine() error {
	return gui.selectSetting(gui.State.Settings.selected + 1)
}

func (gui *Gui) handleSettingsClick() error {
	_, cy := gui.Views.Settings.Cursor()
	_, oy := gui.Views.Settings.Origin()

	if gui.currentViewName() != "settings" {
		if err := gui.switchFocus(gui.Views.Settings); err != nil {
			return err
		}
	}

	return gui.selectSetting(cy + oy)
}

func (gui *Gui) selectSetting(idx int) error {
	count := len(gui.settings())
	if idx < 0 || idx > count-1 {
		return nil
	}

	gui.State.Settings.selected = idx
	gui.renderSettings()

	return nil
}

func (gui *Gui) handleSettingsToggle() error {
	all := gui.settings()
	if gui.State.Settings.selected < 0 || gui.State.Settings.selected >= len(all) {
		return nil
	}
	selected := all[gui.State.Settings.selected]

	var save error
	switch {
	case selected.isSeconds():
		value := nextSeconds(selected.seconds, selected.getInt(&gui.Config.User))
		selected.setInt(&gui.Config.User, value)
		save = config.SetIntSetting(selected.path, value)
	case selected.isChoice():
		choices := selected.choices()
		if len(choices) == 0 {
			return nil
		}
		value := nextChoice(choices, selected.getChoice(&gui.Config.User))
		selected.setChoice(&gui.Config.User, value)
		save = config.SetStringSetting(selected.path, value)
	default:
		value := !selected.get(&gui.Config.User)
		selected.set(&gui.Config.User, value)
		save = config.SetBoolSetting(selected.path, value)
	}

	gui.renderSettings()
	gui.refreshChatTitles()

	// The screen has already changed, so a failed write is reported rather than swallowed: the setting is live but won't outlast the session.
	if save != nil {
		return gui.createErrorPanel("Could not save " + config.ConfigFilename() + ":\n\n" + save.Error() + "\n\nThe change applies to this session only.")
	}

	return nil
}

func nextChoice(choices []string, current string) string {
	for i, choice := range choices {
		if choice == current {
			return choices[(i+1)%len(choices)]
		}
	}

	return choices[0]
}

// nextSeconds advances to the next rung of the ladder, landing on the first value ABOVE a config-file number the ladder does not contain.
// Snapping to the first rung instead would send a hand-edited 45 back to 0 (off) on one keypress, which is a setting silently discarded rather than changed.
func nextSeconds(ladder []int, current int) int {
	for _, value := range ladder {
		if value > current {
			return value
		}
	}

	return ladder[0]
}

func (gui *Gui) handleSettingsEditFile() error {
	return gui.handleOpenConfig(gui.g, gui.Views.Settings)
}

func (gui *Gui) renderSettings() {
	if !gui.State.Settings.active {
		return
	}

	all := gui.settings()
	rows := make([][]string, 0, len(all))
	for _, item := range all {
		rows = append(rows, []string{gui.settingValueLabel(item), item.name, gui.settingHelp(item)})
	}
	selected := gui.State.Settings.selected

	version := presentation.VersionLine(gui.Version, gui.latestVersion, gui.updateState)

	gui.g.Update(func(*gocui.Gui) error {
		view := gui.Views.Settings
		content, err := renderSettingsTable(rows)
		if err != nil {
			return err
		}
		if err := gui.setViewContent(view, content+"\n\n"+version); err != nil {
			return err
		}
		gui.FocusY(selected, len(rows), view)
		return nil
	})
}

func renderSettingsTable(rows [][]string) (string, error) {
	return utils.RenderTable(rows)
}

func (gui *Gui) settingValueLabel(item setting) string {
	if item.isSeconds() {
		return " " + secondsLabel(item.getInt(&gui.Config.User))
	}
	if !item.isChoice() {
		if item.get(&gui.Config.User) {
			return " [on] "
		}
		return " [off]"
	}

	value := item.getChoice(&gui.Config.User)
	if value == "" {
		value = item.emptyHint
	}

	return " " + value
}

// secondsLabel names the disabled state rather than showing a bare 0, which reads as an interval of no length instead of no refresh at all.
func secondsLabel(seconds int) string {
	if seconds <= 0 {
		return "off"
	}

	return fmt.Sprintf("%ds", seconds)
}

func (gui *Gui) settingHelp(item setting) string {
	if item.isSeconds() {
		return fmt.Sprintf("%s — space cycles %d", item.help, len(item.seconds))
	}
	if !item.isChoice() {
		return item.help
	}

	count := len(item.choices())
	switch {
	case count > 1:
		return fmt.Sprintf("%s — space cycles %d", item.help, count)
	case gui.chatModelsError() != "":
		return item.help + " — could not list them: " + gui.chatModelsError()
	default:
		return item.help
	}
}

func (gui *Gui) chatModelsError() string {
	gui.State.Settings.mu.Lock()
	defer gui.State.Settings.mu.Unlock()

	return gui.State.Settings.modelsErr
}

func (gui *Gui) renderSettingsOptions() error {
	return gui.renderOptionsMap(map[string]string{
		"space/enter": "Toggle",
		"↑ ↓":         "Select",
		"e":           "Edit config file",
		"esc":         "Dashboard",
	})
}
