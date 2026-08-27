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
	"github.com/noelruault/lazyaws/ui/types"
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
		{"ECR", func() error { return gui.openActionsMenu("ECR", gui.ECRActions()) }, []string{"Preview lifecycle policy (dry run)"}},
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

	message := waitForView(t, g, gui.Views.Confirmation, "read-only mode")
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
