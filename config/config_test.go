package config

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
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
