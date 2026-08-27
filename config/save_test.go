package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetBoolSettingKeepsTheRestOfTheFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "lazyaws", "config.yml")

	original := `# my lazyaws config
gui:
  scrollHeight: 5      # how fast PgUp/PgDn move
  border: double
somethingLazyawsDoesNotKnowAbout: 42
`
	writeFile(t, path, original)

	if err := SetBoolSetting([]string{"chat", "enabled"}, true); err != nil {
		t.Fatalf("SetBoolSetting() error = %v", err)
	}

	saved := readFile(t, path)
	for _, want := range []string{
		"# my lazyaws config",
		"# how fast PgUp/PgDn move",
		"scrollHeight: 5",
		"border: double",
		"somethingLazyawsDoesNotKnowAbout: 42",
	} {
		if !strings.Contains(saved, want) {
			t.Errorf("saved config lost %q:\n%s", want, saved)
		}
	}

	loaded, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if !loaded.Chat.Enabled {
		t.Error("Chat.Enabled = false after writing it true")
	}
	if loaded.Gui.ScrollHeight != 5 {
		t.Errorf("Gui.ScrollHeight = %d, want the file's 5", loaded.Gui.ScrollHeight)
	}
}

// An int key written as a quoted string leaves a file the next load rejects, so the round trip through LoadUserConfig is the assertion that matters here rather than the bytes.
func TestSetIntSettingWritesANumberTheNextLoadCanRead(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "lazyaws", "config.yml")

	writeFile(t, path, "# keep me\nrefresh:\n  ecsLogsSeconds: 9\n")

	if err := SetIntSetting([]string{"refresh", "metricsSeconds"}, 300); err != nil {
		t.Fatalf("SetIntSetting() error = %v", err)
	}

	saved := readFile(t, path)
	// Unquoted: `metricsSeconds: "300"` is a !!str and an int field will not unmarshal from it.
	if !strings.Contains(saved, "metricsSeconds: 300") {
		t.Errorf("saved config does not hold an unquoted number:\n%s", saved)
	}
	if strings.Contains(saved, `"300"`) || strings.Contains(saved, "'300'") {
		t.Errorf("saved config quoted the number, which the next load cannot parse into an int:\n%s", saved)
	}

	loaded, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() after SetIntSetting() error = %v", err)
	}
	if loaded.Refresh.MetricsSeconds != 300 {
		t.Errorf("Refresh.MetricsSeconds = %d, want the written 300", loaded.Refresh.MetricsSeconds)
	}
	if loaded.Refresh.ECSLogsSeconds != 9 {
		t.Errorf("Refresh.ECSLogsSeconds = %d, want the file's 9 left alone", loaded.Refresh.ECSLogsSeconds)
	}
	if !strings.Contains(saved, "# keep me") {
		t.Errorf("saved config lost its comment:\n%s", saved)
	}
}

// 0 is how a refresh tier is turned off, so it has to survive a write: a writer that treated it as an absent value would make the off state unreachable from the Settings screen.
func TestSetIntSettingWritesZero(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	if err := SetIntSetting([]string{"refresh", "panelSeconds"}, 0); err != nil {
		t.Fatalf("SetIntSetting() error = %v", err)
	}

	loaded, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if loaded.Refresh.PanelSeconds != 0 {
		t.Errorf("Refresh.PanelSeconds = %d, want the written 0", loaded.Refresh.PanelSeconds)
	}
}

func TestSetBoolSettingCreatesTheFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	if err := SetBoolSetting([]string{"readOnly"}, true); err != nil {
		t.Fatalf("SetBoolSetting() error = %v", err)
	}

	loaded, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if !loaded.ReadOnly {
		t.Error("ReadOnly = false after writing it true")
	}

	info, err := os.Stat(ConfigFilename())
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", ConfigFilename(), err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("config file mode = %o, want 644", perm)
	}
}

func TestSetBoolSettingOverwrites(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	for _, value := range []bool{true, false, true, false} {
		if err := SetBoolSetting([]string{"chat", "enabled"}, value); err != nil {
			t.Fatalf("SetBoolSetting(%v) error = %v", value, err)
		}

		loaded, err := LoadUserConfig()
		if err != nil {
			t.Fatalf("LoadUserConfig() error = %v", err)
		}
		if loaded.Chat.Enabled != value {
			t.Errorf("Chat.Enabled = %v, want %v", loaded.Chat.Enabled, value)
		}
	}

	if got := strings.Count(readFile(t, ConfigFilename()), "enabled"); got != 1 {
		t.Errorf("the key appears %d times, want it written once", got)
	}
}

func TestSetBoolSettingReplacesAWrongShapedKey(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeFile(t, filepath.Join(dir, "lazyaws", "config.yml"), "chat: true\n")

	if err := SetBoolSetting([]string{"chat", "enabled"}, true); err != nil {
		t.Fatalf("SetBoolSetting() error = %v", err)
	}

	loaded, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if !loaded.Chat.Enabled {
		t.Error("Chat.Enabled = false, want the reshaped key to hold the value")
	}
}

func TestSetBoolSettingRejectsNonMappingConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeFile(t, filepath.Join(dir, "lazyaws", "config.yml"), "- this is a list\n")

	if err := SetBoolSetting([]string{"readOnly"}, true); err == nil {
		t.Error("SetBoolSetting() error = nil, want a refusal to rewrite a config that isn't a mapping")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}

	return string(data)
}

func TestSetBoolSettingDropsRenamedKeys(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "lazyaws", "config.yml")

	writeFile(t, path, `# kept
amazonQ:
    enabled: true
chat:
    enabled: true
somethingLazyawsDoesNotKnowAbout: 42
`)

	if err := SetBoolSetting([]string{"chat", "enabled"}, false); err != nil {
		t.Fatalf("SetBoolSetting() error = %v", err)
	}

	saved := readFile(t, path)
	if strings.Contains(saved, "amazonQ") {
		t.Errorf("saved config still has the renamed-away key:\n%s", saved)
	}
	for _, want := range []string{"# kept", "chat:", "somethingLazyawsDoesNotKnowAbout: 42"} {
		if !strings.Contains(saved, want) {
			t.Errorf("saved config lost %q:\n%s", want, saved)
		}
	}

	loaded, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if loaded.Chat.Enabled {
		t.Error("Chat.Enabled = true after writing it false")
	}
}
