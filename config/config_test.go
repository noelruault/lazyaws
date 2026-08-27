package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		env  string
		args []string
		want Config
	}{
		{name: "defaults", want: Config{}},
		{name: "version flag", args: []string{"-version"}, want: Config{ShowVersion: true}},
		{name: "debug flag", args: []string{"-debug"}, want: Config{Debug: true}},
		{name: "region round-trips", args: []string{"-region", "eu-west-1"}, want: Config{Region: "eu-west-1"}},
		{name: "region defaults to AWS_REGION", env: "us-east-1", want: Config{Region: "us-east-1"}},
		{name: "region flag overrides AWS_REGION", env: "us-east-1", args: []string{"-region", "eu-west-1", "-version"}, want: Config{Region: "eu-west-1", ShowVersion: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AWS_REGION", tt.env)
			fs := flag.NewFlagSet("lazyaws", flag.ContinueOnError)
			got := parse(fs, tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parse(%v) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

func TestLoadUserConfigMissingFileReturnsDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	got, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if !reflect.DeepEqual(got, DefaultUserConfig()) {
		t.Errorf("LoadUserConfig() = %+v, want defaults %+v", got, DefaultUserConfig())
	}
}

func TestLoadUserConfigPartialFileMergesOverDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeFile(t, filepath.Join(dir, "lazyaws", "config.yml"), "gui:\n  scrollHeight: 5\nconfirmOnQuit: true\n")

	got, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if got.Gui.ScrollHeight != 5 {
		t.Errorf("Gui.ScrollHeight = %d, want 5 (overridden)", got.Gui.ScrollHeight)
	}
	if !got.ConfirmOnQuit {
		t.Error("ConfirmOnQuit = false, want true (overridden)")
	}
	if got.Gui.SidePanelWidth != DefaultUserConfig().Gui.SidePanelWidth {
		t.Errorf("Gui.SidePanelWidth = %v, want default %v (untouched key)", got.Gui.SidePanelWidth, DefaultUserConfig().Gui.SidePanelWidth)
	}
}

// An omitted overviewSeconds must keep the 2s default, while an explicit 0 must survive as 0, because 0 is how a user turns the overview's auto-refresh off.
func TestOverviewSecondsDefaultsToTwoAndZeroSurvives(t *testing.T) {
	if got := DefaultUserConfig().Refresh.OverviewSeconds; got != 2 {
		t.Errorf("DefaultUserConfig().Refresh.OverviewSeconds = %d, want 2", got)
	}

	t.Run("key absent from the file", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		writeFile(t, filepath.Join(dir, "lazyaws", "config.yml"), "refresh:\n  ecsLogsSeconds: 9\n")

		got, err := LoadUserConfig()
		if err != nil {
			t.Fatalf("LoadUserConfig() error = %v", err)
		}
		if got.Refresh.OverviewSeconds != 2 {
			t.Errorf("Refresh.OverviewSeconds = %d, want 2 (untouched key)", got.Refresh.OverviewSeconds)
		}
	})

	t.Run("explicitly zero", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		writeFile(t, filepath.Join(dir, "lazyaws", "config.yml"), "refresh:\n  overviewSeconds: 0\n")

		got, err := LoadUserConfig()
		if err != nil {
			t.Fatalf("LoadUserConfig() error = %v", err)
		}
		if got.Refresh.OverviewSeconds != 0 {
			t.Errorf("Refresh.OverviewSeconds = %d, want 0 (auto-refresh off)", got.Refresh.OverviewSeconds)
		}
	})
}

// The two new tiers follow the same rule as overviewSeconds: an omitted key keeps its default, an explicit 0 means off.
func TestPanelAndMetricsSecondsDefaultAndZeroSurvives(t *testing.T) {
	defaults := DefaultUserConfig().Refresh
	if defaults.PanelSeconds != 2 {
		t.Errorf("DefaultUserConfig().Refresh.PanelSeconds = %d, want 2", defaults.PanelSeconds)
	}
	if defaults.MetricsSeconds != 60 {
		t.Errorf("DefaultUserConfig().Refresh.MetricsSeconds = %d, want 60", defaults.MetricsSeconds)
	}

	t.Run("keys absent from the file", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		writeFile(t, filepath.Join(dir, "lazyaws", "config.yml"), "refresh:\n  ecsLogsSeconds: 9\n")

		got, err := LoadUserConfig()
		if err != nil {
			t.Fatalf("LoadUserConfig() error = %v", err)
		}
		if got.Refresh.PanelSeconds != 2 {
			t.Errorf("Refresh.PanelSeconds = %d, want 2 (untouched key)", got.Refresh.PanelSeconds)
		}
		if got.Refresh.MetricsSeconds != 60 {
			t.Errorf("Refresh.MetricsSeconds = %d, want 60 (untouched key)", got.Refresh.MetricsSeconds)
		}
	})

	t.Run("explicitly zero", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		writeFile(t, filepath.Join(dir, "lazyaws", "config.yml"), "refresh:\n  panelSeconds: 0\n  metricsSeconds: 0\n")

		got, err := LoadUserConfig()
		if err != nil {
			t.Fatalf("LoadUserConfig() error = %v", err)
		}
		if got.Refresh.PanelSeconds != 0 {
			t.Errorf("Refresh.PanelSeconds = %d, want 0 (auto-refresh off)", got.Refresh.PanelSeconds)
		}
		if got.Refresh.MetricsSeconds != 0 {
			t.Errorf("Refresh.MetricsSeconds = %d, want 0 (auto-refresh off)", got.Refresh.MetricsSeconds)
		}
	})
}

// CloudWatch bills per metric per GetMetricData request, so the floor is the one refresh setting the app overrides rather than obeys.
// Applied on read, so the number the user wrote stays in the file: the clamp must not be visible as a rewritten key.
func TestMetricsIntervalAppliesItsFloorWithoutRewritingTheSetting(t *testing.T) {
	tests := []struct {
		seconds int
		want    time.Duration
	}{
		{seconds: 0, want: 0},
		{seconds: -5, want: 0},
		{seconds: 1, want: 10 * time.Second},
		{seconds: 9, want: 10 * time.Second},
		{seconds: 10, want: 10 * time.Second},
		{seconds: 60, want: 60 * time.Second},
		{seconds: 300, want: 300 * time.Second},
	}

	for _, test := range tests {
		refresh := RefreshConfig{MetricsSeconds: test.seconds}
		if got := refresh.MetricsInterval(); got != test.want {
			t.Errorf("RefreshConfig{MetricsSeconds: %d}.MetricsInterval() = %v, want %v", test.seconds, got, test.want)
		}
		// The field is what the file holds and what the Settings row shows, so the floor must not have moved it.
		if refresh.MetricsSeconds != test.seconds {
			t.Errorf("MetricsInterval() rewrote MetricsSeconds to %d, want the configured %d left alone", refresh.MetricsSeconds, test.seconds)
		}
	}

	if MetricsFloorSeconds != 10 {
		t.Errorf("MetricsFloorSeconds = %d, want 10", MetricsFloorSeconds)
	}
}

// Every refresh key is published twice, in DefaultUserConfig and in README.md's sample block, and nothing else pins them to each other.
// The keybindings table has TestReadmeKeyTableIsCurrent for exactly this reason; the sample block had nothing, so a changed default left the documented sample quietly wrong.
func TestReadmeRefreshSampleMatchesTheDefaults(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "README.md"))
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}

	refresh := DefaultUserConfig().Refresh
	for key, value := range map[string]int{
		"ecsLogsSeconds":   refresh.ECSLogsSeconds,
		"ec2StatusSeconds": refresh.EC2StatusSeconds,
		"overviewSeconds":  refresh.OverviewSeconds,
		"panelSeconds":     refresh.PanelSeconds,
		"metricsSeconds":   refresh.MetricsSeconds,
	} {
		want := fmt.Sprintf("%s: %d", key, value)
		if !strings.Contains(string(readme), want) {
			t.Errorf("README.md's sample block does not document %q; it publishes the default a second time and has drifted", want)
		}
	}

	// The floor is a third publication of the same number: the constant, the Settings ladder's first rung, and this sentence.
	if want := fmt.Sprintf("anything under %d is treated as %d", MetricsFloorSeconds, MetricsFloorSeconds); !strings.Contains(string(readme), want) {
		t.Errorf("README.md does not state the metrics floor as %q", want)
	}
}

func TestReadOnlyIsOffByDefault(t *testing.T) {
	if DefaultUserConfig().ReadOnly {
		t.Error("DefaultUserConfig() turns read-only mode on, want every action offered by default")
	}

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	got, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if got.ReadOnly {
		t.Error("ReadOnly = true with no config file, want false")
	}
}

func TestChatIsOffUnlessAskedFor(t *testing.T) {
	if DefaultUserConfig().Chat.Enabled {
		t.Error("DefaultUserConfig() enables the Amazon Q chat, want it off")
	}

	t.Run("no config file", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())

		got, err := LoadUserConfig()
		if err != nil {
			t.Fatalf("LoadUserConfig() error = %v", err)
		}
		if got.Chat.Enabled {
			t.Error("Chat.Enabled = true with no config file, want false")
		}
	})

	t.Run("config file that says nothing about it", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		writeFile(t, filepath.Join(dir, "lazyaws", "config.yml"), "gui:\n  scrollHeight: 5\n")

		got, err := LoadUserConfig()
		if err != nil {
			t.Fatalf("LoadUserConfig() error = %v", err)
		}
		if got.Chat.Enabled {
			t.Error("Chat.Enabled = true from an unrelated config, want false")
		}
	})

	t.Run("turned on", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		writeFile(t, filepath.Join(dir, "lazyaws", "config.yml"), "chat:\n  enabled: true\n")

		got, err := LoadUserConfig()
		if err != nil {
			t.Fatalf("LoadUserConfig() error = %v", err)
		}
		if !got.Chat.Enabled {
			t.Error("Chat.Enabled = false after asking for it, want true")
		}
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
