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
