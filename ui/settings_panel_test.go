package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jesseduffield/gocui"
	"github.com/noelruault/lazyaws/ui/layout"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/config"
	"github.com/noelruault/lazyaws/ui/presentation"
	"github.com/noelruault/lazyaws/ui/types"
	"github.com/noelruault/lazyaws/ui/utils"
)

func TestSettingsScreenOpensAndCloses(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, gui.handleToggleSettings)

	if name := ask(g, func() string { return g.CurrentView().Name() }); name != "settings" {
		t.Errorf("focused view = %q, want the settings list", name)
	}
	windows := ask(g, func() map[string]layout.Dimensions { return gui.getWindowDimensions("", "") })
	if _, ok := windows["settings"]; !ok {
		t.Error("the settings window is not laid out while the screen is up")
	}
	if _, ok := windows["profile"]; ok {
		t.Error("dashboard windows are still laid out while the settings screen is up")
	}

	listing := waitForView(t, g, gui.Views.Settings, "Chat backend")
	for _, want := range []string{"Chat", "Chat model", "Read-only mode"} {
		if !strings.Contains(listing, want) {
			t.Errorf("settings = %q, want a %q row", listing, want)
		}
	}
	if !strings.Contains(listing, "[on]") {
		t.Errorf("settings = %q, want the chat switch shown as on (the harness enables it)", listing)
	}
	if !strings.Contains(listing, "[off]") {
		t.Errorf("settings = %q, want read-only shown as off", listing)
	}

	run(t, g, gui.handleExitSettings)

	if ask(g, func() bool { return gui.State.Settings.active }) {
		t.Error("Settings.active = true after exiting")
	}
	if _, ok := ask(g, func() map[string]layout.Dimensions { return gui.getWindowDimensions("", "") })["profile"]; !ok {
		t.Error("dashboard windows are not laid out after leaving the settings screen")
	}
}

// Toggles must update both the running configuration and disk.
func TestTogglingASettingWritesTheConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	gui, g := newHeadlessGuiWithConfig(t, config.DefaultUserConfig())

	run(t, g, gui.handleToggleSettings)
	run(t, g, gui.handleSettingsToggle) // the first switch is the Amazon Q chat

	if !gui.qEnabled() {
		t.Error("the running config still has the Amazon Q chat off after toggling it on")
	}

	saved, err := config.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if !saved.Chat.Enabled {
		t.Error("the config file still has the Amazon Q chat off after toggling it on")
	}

	if listing := readView(g, gui.Views.Settings); !strings.Contains(listing, "[on]") {
		t.Errorf("settings = %q, want the switch shown as on", listing)
	}

	run(t, g, gui.handleSettingsToggle)

	saved, err = config.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if saved.Chat.Enabled {
		t.Error("the config file still has the chat on after toggling it off")
	}
}

func TestTogglingReadOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	gui, g := newHeadlessGuiWithConfig(t, config.DefaultUserConfig())

	run(t, g, gui.handleToggleSettings)
	selectSettingNamed(t, g, gui, "Read-only mode")
	run(t, g, gui.handleSettingsToggle)

	if !gui.readOnly() {
		t.Error("readOnly() = false after toggling read-only mode on")
	}

	saved, err := config.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if !saved.ReadOnly {
		t.Error("the config file does not have readOnly set")
	}
}

// A failed write must remain visible after the live setting changes.
func TestTogglingReportsAFailedWrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "lazyaws", "config.yml"), 0o755); err != nil {
		t.Fatal(err)
	}

	gui, g := newHeadlessGuiWithConfig(t, config.DefaultUserConfig())

	run(t, g, gui.handleToggleSettings)
	run(t, g, gui.handleSettingsToggle)

	message := waitForView(t, g, gui.Views.Confirmation, "Could not save")
	if !strings.Contains(message, "this session only") {
		t.Errorf("message = %q, want it to say the change won't outlast the session", message)
	}
}

// An interval row cycles its ladder and writes a NUMBER: a string write would leave a file the next load rejects on the key the screen had just saved.
func TestCyclingARefreshInterval(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	user := config.DefaultUserConfig()
	gui, g := newHeadlessGuiWithConfig(t, user)

	run(t, g, gui.handleToggleSettings)
	selectSettingNamed(t, g, gui, "Panel refresh")

	if got := gui.Config.User.Refresh.PanelSeconds; got != 2 {
		t.Fatalf("PanelSeconds = %d, want the default 2", got)
	}

	run(t, g, gui.handleSettingsToggle)
	if got := gui.Config.User.Refresh.PanelSeconds; got != 5 {
		t.Errorf("PanelSeconds = %d after one step, want the next rung 5", got)
	}

	saved, err := config.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() after the write error = %v", err)
	}
	if saved.Refresh.PanelSeconds != 5 {
		t.Errorf("saved PanelSeconds = %d, want 5", saved.Refresh.PanelSeconds)
	}

	// Past the top rung it wraps to off, which is what makes the disabled state reachable from the screen at all.
	run(t, g, gui.handleSettingsToggle)
	run(t, g, gui.handleSettingsToggle)
	if got := gui.Config.User.Refresh.PanelSeconds; got != 0 {
		t.Errorf("PanelSeconds = %d after wrapping past the top rung, want 0", got)
	}
}

// The metrics ladder starts at the floor, because CloudWatch bills per metric per request and the screen must not offer a rate the app would then refuse.
func TestTheMetricsLadderNeverOffersLessThanTheFloor(t *testing.T) {
	gui, _ := newHeadlessGuiWithConfig(t, config.DefaultUserConfig())

	for _, item := range gui.settings() {
		if item.name != "Metrics refresh" {
			continue
		}
		for _, seconds := range item.seconds {
			if seconds != 0 && seconds < config.MetricsFloorSeconds {
				t.Errorf("the metrics ladder offers %ds, below the %ds floor MetricsInterval enforces", seconds, config.MetricsFloorSeconds)
			}
		}
		return
	}

	t.Fatal("no \"Metrics refresh\" row on the settings screen")
}

// A row's rendered value has to say what the setting IS: a bare 0 reads as an interval of no length rather than as no refresh at all.
func TestSecondsLabelNamesTheDisabledState(t *testing.T) {
	if got := secondsLabel(0); got != "off" {
		t.Errorf("secondsLabel(0) = %q, want %q", got, "off")
	}
	if got := secondsLabel(60); got != "60s" {
		t.Errorf("secondsLabel(60) = %q, want %q", got, "60s")
	}
}

// A hand-edited number the ladder does not contain must step UP to the next rung, not snap back to the first: snapping would send 45 to off on one keypress, discarding the setting rather than changing it.
func TestNextSecondsStepsUpFromAValueOffTheLadder(t *testing.T) {
	ladder := []int{0, 1, 2, 5, 10}

	tests := []struct {
		current int
		want    int
	}{
		{current: 0, want: 1},
		{current: 2, want: 5},
		{current: 3, want: 5},
		{current: 10, want: 0},
		{current: 45, want: 0},
	}

	for _, test := range tests {
		if got := nextSeconds(ladder, test.current); got != test.want {
			t.Errorf("nextSeconds(%v, %d) = %d, want %d", ladder, test.current, got, test.want)
		}
	}
}

func TestCyclingTheChatBackend(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	gui, g := newHeadlessGuiWithConfig(t, config.DefaultUserConfig())

	run(t, g, gui.handleToggleSettings)
	selectSettingNamed(t, g, gui, "Chat backend")

	if got := gui.chatProvider(); got != config.ProviderBedrock {
		t.Fatalf("provider = %q, want the default %q", got, config.ProviderBedrock)
	}

	run(t, g, gui.handleSettingsToggle)
	if got := gui.chatProvider(); got != config.ProviderKiro {
		t.Errorf("provider = %q after one step, want %q", got, config.ProviderKiro)
	}
	saved, err := config.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if saved.Chat.Provider != config.ProviderKiro {
		t.Errorf("saved provider = %q, want %q", saved.Chat.Provider, config.ProviderKiro)
	}

	run(t, g, gui.handleSettingsToggle)
	if got := gui.chatProvider(); got != config.ProviderBedrock {
		t.Errorf("provider = %q after wrapping, want %q", got, config.ProviderBedrock)
	}
}

// Runtime model discovery must remain selectable without a release.
func TestCyclingTheChatModel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	gui, g := newHeadlessGuiWithConfig(t, config.DefaultUserConfig())

	discovered := []string{"anthropic.claude-haiku-4-5-20251001-v1:0", "anthropic.claude-sonnet-4-6", "amazon.nova-micro-v1:0"}
	run(t, g, func() error {
		gui.State.Settings.mu.Lock()
		gui.State.Settings.models = discovered
		gui.State.Settings.mu.Unlock()
		return nil
	})

	run(t, g, gui.handleToggleSettings)
	selectSettingNamed(t, g, gui, "Chat model")

	if got := gui.chatModel(); got != config.DefaultChatModel {
		t.Fatalf("model = %q, want the default %q", got, config.DefaultChatModel)
	}

	seen := map[string]bool{}
	for range discovered {
		run(t, g, gui.handleSettingsToggle)
		seen[gui.chatModel()] = true
	}
	for _, want := range discovered {
		if !seen[want] {
			t.Errorf("cycling never landed on %q; visited %v", want, seen)
		}
	}

	saved, err := config.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if saved.Chat.Model != gui.chatModel() {
		t.Errorf("saved model = %q, want the selected %q", saved.Chat.Model, gui.chatModel())
	}

	if listing := readView(g, gui.Views.Settings); !strings.Contains(listing, "space cycles 3") {
		t.Errorf("settings = %q, want the choice count shown", listing)
	}
}

// Discovery failure must not drop the configured model.
func TestChatModelChoicesAlwaysIncludeTheConfiguredOne(t *testing.T) {
	gui, _ := newHeadlessGuiWithConfig(t, config.DefaultUserConfig())

	choices := gui.chatModelChoices()
	if len(choices) != 1 || choices[0] != config.DefaultChatModel {
		t.Errorf("choices = %v, want just the configured %q", choices, config.DefaultChatModel)
	}
}

func TestNextChoiceWraps(t *testing.T) {
	choices := []string{"a", "b", "c"}

	tests := []struct{ current, want string }{
		{"a", "b"},
		{"c", "a"},
		{"not in the list", "a"},
		{"", "a"},
	}
	for _, tt := range tests {
		if got := nextChoice(choices, tt.current); got != tt.want {
			t.Errorf("nextChoice(%q) = %q, want %q", tt.current, got, tt.want)
		}
	}
}

// Explicit survivor sets catch new actions that omit their mutating marker.
func TestReadOnlyHidesMutatingMenuItems(t *testing.T) {
	user := config.DefaultUserConfig()
	user.ReadOnly = true
	gui, _ := newHeadlessGuiWithConfig(t, user)

	tests := []struct {
		name  string
		items []*types.MenuItem
		want  []string
	}{
		{
			name: "EC2",
			items: []*types.MenuItem{
				{Label: "Start", Mutates: true},
				{Label: "Terminate", Mutates: true},
				{Label: "Connect via EC2 Instance Connect", Mutates: true},
			},
			want: nil,
		},
		{
			name: "ECR keeps the dry run",
			items: []*types.MenuItem{
				{Label: "Delete repository", Mutates: true},
				{Label: "Preview lifecycle policy (dry run)"},
			},
			want: []string{"Preview lifecycle policy (dry run)"},
		},
		{
			name: "S3 object menu is all reads",
			items: []*types.MenuItem{
				{Label: "Versions"},
				{Label: "Presigned URL (1h)"},
			},
			want: []string{"Versions", "Presigned URL (1h)"},
		},
	}

	for _, tt := range tests {
		kept := gui.dropMutatingItems(tt.items)

		var labels []string
		for _, item := range kept {
			labels = append(labels, item.Label)
		}
		if strings.Join(labels, "|") != strings.Join(tt.want, "|") {
			t.Errorf("%s: kept %q, want %q", tt.name, labels, tt.want)
		}
	}
}

func TestMenusAreWholeWhenNotReadOnly(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	items := []*types.MenuItem{{Label: "Start", Mutates: true}, {Label: "Versions"}}
	if got := len(gui.dropMutatingItems(items)); got != 2 {
		t.Errorf("kept %d items, want both", got)
	}
}

// The real menus must mark every AWS mutation for read-only filtering.
func TestShippedActionMenusMarkTheirMutatingItems(t *testing.T) {
	user := config.DefaultUserConfig()
	user.ReadOnly = true
	gui, g := newHeadlessGuiWithConfig(t, user)

	gui.Panels.EC2.SetItems([]*aws.Instance{{ID: "i-1", Name: "web"}})
	gui.Panels.S3.SetItems([]*aws.Bucket{{Name: "bucket"}})
	gui.Panels.EKS.SetItems([]*aws.EKSCluster{{Name: "cluster", Version: "1.29"}})
	gui.Panels.ECR.SetItems([]*aws.ECRRepository{{Name: "repo"}})
	gui.Panels.Secrets.SetItems([]*aws.SecretSummary{{Name: "secret"}})

	menus := []struct {
		name string
		open func() error
		want []string
	}{
		{"EC2", func() error { return gui.openActionsMenu("EC2", gui.EC2Actions()) }, nil},
		{"S3", func() error { return gui.openActionsMenu("S3", gui.S3Actions()) }, nil},
		{"EKS", func() error { return gui.openActionsMenu("EKS", gui.EKSActions()) }, nil},
		// The lifecycle preview left this menu when the guard arrived: it changes no policy, but StartLifecyclePolicyPreview is still a call asking ECR to make something, and a read-only session promises none are made.
		{"ECR", func() error { return gui.openActionsMenu("ECR", gui.ECRActions()) }, nil},
		{"Secrets", func() error { return gui.openActionsMenu("Secrets", gui.SecretsActions()) }, []string{"View value"}},
	}

	for _, menu := range menus {
		run(t, g, menu.open)

		var labels []string
		for _, item := range ask(g, func() []*types.MenuItem { return gui.Panels.Menu.List.GetItems() }) {
			label := item.Label
			if label == "" && len(item.LabelColumns) > 0 {
				label = item.LabelColumns[0]
			}
			if label == "cancel" {
				continue
			}
			labels = append(labels, label)
		}

		if strings.Join(labels, "|") != strings.Join(menu.want, "|") {
			t.Errorf("%s menu in read-only mode offers %q, want %q (mark new mutating items with Mutates: true)", menu.name, labels, menu.want)
		}

		run(t, g, gui.handleMenuClose)
	}
}

// Read-only mode must refuse tool-enabled Kiro with actionable guidance.
func TestReadOnlyRefusesTheCLIBackend(t *testing.T) {
	fakeQOnPath(t, "exit 0")
	user := config.DefaultUserConfig()
	user.ReadOnly = true
	user.Chat.Enabled = true
	user.Chat.Provider = config.ProviderKiro
	gui, g := newHeadlessGuiWithConfig(t, user)

	run(t, g, gui.handleToggleQ)

	message := waitForView(t, g, gui.Views.Confirmation, "Read-only mode is on")
	if !strings.Contains(message, "--trust-all-tools") {
		t.Errorf("message = %q, want the actual reason: the CLI runs commands", message)
	}
	if ask(g, gui.qScreenActive) {
		t.Error("the CLI-backed chat opened in read-only mode")
	}

	run(t, g, func() error { return gui.askQ("do something") })
	if got := ask(g, func() int { return len(gui.State.Q.chats) }); got != 0 {
		t.Errorf("chats = %d, want none in read-only mode", got)
	}
}

// Bedrock has no tools, so read-only mode must leave it available.
func TestReadOnlyAllowsTheBedrockBackend(t *testing.T) {
	user := config.DefaultUserConfig()
	user.ReadOnly = true
	user.Chat.Enabled = true
	gui, g := newHeadlessGuiWithConfig(t, user)

	run(t, g, gui.handleToggleQ)

	if !ask(g, gui.qScreenActive) {
		t.Error("the Bedrock-backed chat was refused in read-only mode, though it cannot change anything")
	}
}

// The Settings screen is where the build names itself, because the information line has room for a bullet but not for the release the bullet is about.
func TestSettingsScreenNamesTheBuildAndANewerRelease(t *testing.T) {
	gui, g := newHeadlessGui(t)
	run(t, g, func() error {
		gui.Version = "v0.3.0"
		return gui.handleToggleSettings()
	})

	waitForView(t, g, gui.Views.Settings, "lazyaws v0.3.0")

	run(t, g, func() error {
		gui.latestVersion = "v0.4.0"
		gui.updateState = presentation.UpdateOutdated
		gui.renderSettings()
		return nil
	})

	listing := waitForView(t, g, gui.Views.Settings, "(v0.4.0 available)")
	if !strings.Contains(listing, "Read-only mode") {
		t.Errorf("settings = %q, want the version alongside the rows, not instead of them", listing)
	}
}

// Drilling into a resource moves focus to the detail pane, and the pane then describes a row in a list that is no longer focused.
// If the list drops its selection marker there, nothing on screen says which of its rows the pane is about.
func TestAListKeepsItsSelectionMarkedAfterFocusMovesToTheDetailPane(t *testing.T) {
	gui, g := newHeadlessGui(t)

	// onFocusChange is what decides which views draw a highlight, and it runs from the focus manager on every frame; the headless harness registers no managers, so it is driven here the way the loop drives it.
	focus := func(view *gocui.View) {
		t.Helper()
		run(t, g, func() error {
			if err := gui.switchFocus(view); err != nil {
				return err
			}
			return gui.onFocusChange()
		})
	}

	focus(gui.Views.S3)
	if !ask(g, func() bool { return gui.Views.S3.Highlight }) {
		t.Fatal("the focused S3 list does not mark its selected row, so this test cannot see the case it is about")
	}
	if got := ask(g, func() gocui.Attribute { return gui.Views.S3.SelBgColor }); got != gui.selectedLineBgColor {
		t.Errorf("the focused list's selection bar is %v, want the theme's %v", got, gui.selectedLineBgColor)
	}

	focus(gui.Views.Main)
	if !ask(g, func() bool { return gui.Views.S3.Highlight }) {
		t.Error("the S3 list stopped marking its selected row once focus moved to the pane describing it")
	}
	if ask(g, func() bool { return gui.Views.Main.Highlight }) {
		t.Error("the detail pane marks a selected line of its own, which competes with the list's")
	}

	// Exactly one bar on screen is what keeps "which panel takes the keys" answerable: unfocused lists keep the marker gocui draws without a background.
	for _, view := range []*gocui.View{gui.Views.S3, gui.Views.EC2, gui.Views.Profile} {
		if got := ask(g, func() gocui.Attribute { return view.SelBgColor }); got != gocui.ColorDefault {
			t.Errorf("the unfocused %s list still paints a selection bar (%v)", view.Name(), got)
		}
	}
}

// Sizing the information box is not the same as filling it: for a long time the version was measured into a gap at the end of the bottom line and never written there.
func TestLayoutPaintsTheVersionIntoTheInformationCell(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, func() error {
		gui.Version = "v0.3.0"
		gui.updateState = presentation.UpdateCurrent
		return gui.layout(g)
	})

	if got := utils.Decolorise(readView(g, gui.Views.Information)); !strings.Contains(got, "v0.3.0 ●") {
		t.Errorf("information cell = %q, want the version and its bullet", got)
	}
}

// The information line carries the version wherever the user is, so it must render the state the indicator was given rather than the version alone.
func TestInformationLineCarriesTheVersionBullet(t *testing.T) {
	gui := &Gui{Version: "v0.3.0"}

	if got := gui.getInformationContent(); got != "v0.3.0" {
		t.Errorf("unchecked build: getInformationContent() = %q, want the bare version", got)
	}

	gui.updateState = presentation.UpdateOutdated
	if got := utils.Decolorise(gui.getInformationContent()); got != "v0.3.0 ●" {
		t.Errorf("outdated build: getInformationContent() = %q, want %q", got, "v0.3.0 ●")
	}
}

// selectSettingNamed moves the cursor onto a row by name, so these tests survive rows being reordered.
func selectSettingNamed(t *testing.T, g *gocui.Gui, gui *Gui, name string) {
	t.Helper()

	run(t, g, func() error {
		for idx, item := range gui.settings() {
			if item.name == name {
				return gui.selectSetting(idx)
			}
		}
		t.Fatalf("no %q row on the settings screen", name)
		return nil
	})
}
