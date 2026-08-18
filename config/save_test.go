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
