package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Profile section matching must not leak neighboring configuration.
func TestReadAWSConfigSection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[default]\nregion = us-east-1\n\n[profile staging]\nregion = eu-west-1\nrole_arn = arn:aws:iam::123:role/staging\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		profile string
		wantSub string
	}{
		{"default", "region = us-east-1"},
		{"staging", "role_arn = arn:aws:iam::123:role/staging"},
		{"missing", "no config section found for profile missing"},
	}

	for _, tt := range tests {
		got := readAWSConfigSection(tt.profile)
		if !strings.Contains(got, tt.wantSub) {
			t.Errorf("readAWSConfigSection(%q) = %q, want substring %q", tt.profile, got, tt.wantSub)
		}
	}

	if strings.Contains(readAWSConfigSection("default"), "eu-west-1") {
		t.Error("readAWSConfigSection(\"default\") leaked staging's region")
	}
}

// refreshProfile is a reloader: it runs on r/R and on the background refresh, not only at startup.
// It opens the panel on the connected profile, but once the cursor has moved a later refresh must leave it where the user put it.
func TestProfileRefreshOpensOnTheCurrentProfileThenLeavesTheCursorAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".aws"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".aws", "config"), []byte("[profile alpha]\n[profile staging]\n[profile zeta]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	gui, g := newHeadlessGui(t)
	gui.CurrentProfile = "staging"

	run(t, g, gui.refreshProfile)
	if got := ask(g, func() int { return gui.Panels.Profile.SelectedIdx }); got != 1 {
		t.Fatalf("SelectedIdx after the first load = %d, want 1 (the connected profile)", got)
	}

	run(t, g, func() error {
		gui.Panels.Profile.SetSelectedLineIdx(2)
		return gui.refreshProfile()
	})
	if got := ask(g, func() int { return gui.Panels.Profile.SelectedIdx }); got != 2 {
		t.Errorf("SelectedIdx after a refresh = %d, want 2 (the row the cursor was moved to, not the connected profile)", got)
	}
}
