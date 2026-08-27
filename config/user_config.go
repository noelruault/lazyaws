package config

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type UserConfig struct {
	Gui           GuiConfig     `yaml:"gui"`
	ConfirmOnQuit bool          `yaml:"confirmOnQuit"`
	Refresh       RefreshConfig `yaml:"refresh"`
	Chat          ChatConfig    `yaml:"chat"`

	// ReadOnly hides mutating actions and blocks shells and tool-enabled chat backends.
	ReadOnly bool `yaml:"readOnly"`

	Keybindings map[string]string `yaml:"keybindings"`
}

// ChatConfig is opt-in because the Kiro backend can act on AWS with the caller's credentials.
type ChatConfig struct {
	Enabled bool `yaml:"enabled"`

	// Provider is "bedrock" or "kiro"; Kiro requires a separate installation and login.
	Provider string `yaml:"provider"`

	// Model is ignored by Kiro because that backend owns its model selection.
	Model string `yaml:"model"`
}

const (
	ProviderBedrock = "bedrock"
	ProviderKiro    = "kiro"
)

const DefaultChatModel = "anthropic.claude-sonnet-4-6"

type GuiConfig struct {
	ScrollHeight           int     `yaml:"scrollHeight"`
	ScrollPastBottom       bool    `yaml:"scrollPastBottom"`
	SidePanelWidth         float64 `yaml:"sidePanelWidth"`
	ScreenMode             string  `yaml:"screenMode"` // normal|half|fullscreen
	Border                 string  `yaml:"border"`     // rounded|single|double|hidden
	ExpandFocusedSidePanel bool    `yaml:"expandFocusedSidePanel"`
	IgnoreMouseEvents      bool    `yaml:"ignoreMouseEvents"`
	ShowBottomLine         bool    `yaml:"showBottomLine"`
	WrapMainPanel          bool    `yaml:"wrapMainPanel"`

	// DimBehindPopups can be disabled because some terminals barely render the faint attribute.
	DimBehindPopups bool `yaml:"dimBehindPopups"`

	Theme ThemeConfig `yaml:"theme"`
}

type ThemeConfig struct {
	ActiveBorderColor   []string `yaml:"activeBorderColor"`
	InactiveBorderColor []string `yaml:"inactiveBorderColor"`
	SelectedLineBgColor []string `yaml:"selectedLineBgColor"`
	OptionsTextColor    []string `yaml:"optionsTextColor"`
}

type RefreshConfig struct {
	ECSLogsSeconds   int `yaml:"ecsLogsSeconds"`
	EC2StatusSeconds int `yaml:"ec2StatusSeconds"`

	// OverviewSeconds turns the overview's auto-refresh OFF at 0, so it must not go through RefreshInterval, which substitutes a fallback for every non-positive value.
	OverviewSeconds int `yaml:"overviewSeconds"`

	// PanelSeconds is how often the FOCUSED side panel's list reloads; the same 0-means-off rule as OverviewSeconds.
	PanelSeconds int `yaml:"panelSeconds"`

	// MetricsSeconds is a separate, slower tier because GetMetricData is billed per metric requested, so this interval is the one with a price attached. Read it through MetricsInterval, which applies the floor.
	MetricsSeconds int `yaml:"metricsSeconds"`
}

// MetricsFloorSeconds is the shortest metrics refresh the app will use, whatever the config file asks for.
// CloudWatch charges per metric per GetMetricData request and publishes most metrics at 60-second resolution anyway, so a shorter interval buys repeated readings of an unchanged datapoint and bills for each one.
const MetricsFloorSeconds = 10

// MetricsInterval reports how often metrics may be refetched, 0 meaning never.
// The floor is applied on READ rather than clamped on load, so a hand-edited 1 stays visible in the file as what was asked for while the app still refuses to spend at that rate.
func (r RefreshConfig) MetricsInterval() time.Duration {
	if r.MetricsSeconds <= 0 {
		return 0
	}

	return time.Duration(max(r.MetricsSeconds, MetricsFloorSeconds)) * time.Second
}

// DefaultUserConfig starts complete so partial or missing configuration remains usable.
func DefaultUserConfig() UserConfig {
	return UserConfig{
		Gui: GuiConfig{
			ScrollHeight:    2,
			SidePanelWidth:  0.333,
			ScreenMode:      "normal",
			Border:          "rounded",
			ShowBottomLine:  true,
			WrapMainPanel:   true,
			DimBehindPopups: true,
			Theme: ThemeConfig{
				ActiveBorderColor:   []string{"green", "bold"},
				InactiveBorderColor: []string{"default"},
				SelectedLineBgColor: []string{"blue"},
				OptionsTextColor:    []string{"blue"},
			},
		},
		Refresh: RefreshConfig{
			ECSLogsSeconds:   5,
			EC2StatusSeconds: 10,
			OverviewSeconds:  2,
			PanelSeconds:     2,
			MetricsSeconds:   60,
		},
		// Chat is opt-in because Kiro can act on AWS; Bedrock is the default because it needs no second installation or login.
		Chat: ChatConfig{
			Enabled:  false,
			Provider: ProviderBedrock,
			Model:    DefaultChatModel,
		},
		ReadOnly: false,
	}
}

// RefreshInterval substitutes fallbackSeconds because time.NewTicker panics on non-positive durations.
func RefreshInterval(seconds, fallbackSeconds int) time.Duration {
	if seconds <= 0 {
		seconds = fallbackSeconds
	}
	return time.Duration(seconds) * time.Second
}

// ConfigDir honors XDG_CONFIG_HOME even where os.UserConfigDir ignores it.
func ConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "lazyaws")
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "lazyaws")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "lazyaws")
}

func ConfigFilename() string {
	return filepath.Join(ConfigDir(), "config.yml")
}

// LoadUserConfig returns defaults without error when the file is missing.
func LoadUserConfig() (UserConfig, error) {
	cfg := DefaultUserConfig()

	data, err := os.ReadFile(ConfigFilename())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
