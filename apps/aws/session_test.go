package aws

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func configWithCreds(region string, creds aws.Credentials) aws.Config {
	return aws.Config{
		Region: region,
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return creds, nil
		}),
	}
}

func TestSaveCachedCredentialsRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	expires := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
	cfg := configWithCreds("eu-west-1", aws.Credentials{
		AccessKeyID:     "AKIAEXAMPLE",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		CanExpire:       true,
		Expires:         expires,
	})

	if err := saveCachedCredentials(context.Background(), "work", cfg); err != nil {
		t.Fatalf("saveCachedCredentials: %v", err)
	}

	entry, err := loadCachedSession("work")
	if err != nil {
		t.Fatalf("loadCachedSession: %v", err)
	}
	if entry.Profile != "work" {
		t.Errorf("Profile = %q, want %q", entry.Profile, "work")
	}
	if entry.Region != "eu-west-1" {
		t.Errorf("Region = %q, want %q", entry.Region, "eu-west-1")
	}
	if entry.AccessKey != "AKIAEXAMPLE" || entry.SecretKey != "secret" || entry.SessionTok != "token" {
		t.Errorf("credentials did not round-trip: %+v", entry)
	}
	if !entry.ExpiresAt.Equal(expires) {
		t.Errorf("ExpiresAt = %v, want %v", entry.ExpiresAt, expires)
	}
}

func TestSaveCachedCredentialsSkipsNonTemporary(t *testing.T) {
	tests := []struct {
		name  string
		creds aws.Credentials
	}{
		{
			name: "cannot expire",
			creds: aws.Credentials{
				AccessKeyID:     "AKIASTATIC",
				SecretAccessKey: "secret",
				CanExpire:       false,
			},
		},
		{
			name: "zero expiry",
			creds: aws.Credentials{
				AccessKeyID:     "AKIAZERO",
				SecretAccessKey: "secret",
				CanExpire:       true,
				Expires:         time.Time{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)

			if err := saveCachedCredentials(context.Background(), "work", configWithCreds("eu-west-1", tt.creds)); err != nil {
				t.Fatalf("saveCachedCredentials: %v", err)
			}
			if _, err := os.Stat(filepath.Join(home, ".lazyaws", "session")); !os.IsNotExist(err) {
				t.Errorf("expected no cache file, stat err = %v", err)
			}
		})
	}
}

func TestLoadCachedSessionDeterministic(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	file := cachedSessionsFile{Sessions: map[string]cachedSession{
		"alpha": {Profile: "alpha", Region: "eu-west-1", AccessKey: "AKIAALPHA", ExpiresAt: time.Now().Add(time.Hour)},
		"beta":  {Profile: "beta", Region: "us-east-1", AccessKey: "AKIABETA", ExpiresAt: time.Now().Add(time.Hour)},
	}}
	data, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	dir := filepath.Join(home, ".lazyaws")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session"), data, 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	tests := []struct {
		profile    string
		wantRegion string
		wantKey    string
	}{
		{"alpha", "eu-west-1", "AKIAALPHA"},
		{"beta", "us-east-1", "AKIABETA"},
	}

	for _, tt := range tests {
		entry, err := loadCachedSession(tt.profile)
		if err != nil {
			t.Fatalf("loadCachedSession(%q): %v", tt.profile, err)
		}
		if entry.Region != tt.wantRegion {
			t.Errorf("loadCachedSession(%q).Region = %q, want %q", tt.profile, entry.Region, tt.wantRegion)
		}
		if entry.AccessKey != tt.wantKey {
			t.Errorf("loadCachedSession(%q).AccessKey = %q, want %q", tt.profile, entry.AccessKey, tt.wantKey)
		}
	}
}

func TestLoadCachedAWSConfigMisses(t *testing.T) {
	tests := []struct {
		name      string
		envKeyID  string
		expiresAt time.Time
	}{
		{
			name:      "env credentials take precedence",
			envKeyID:  "AKIAENV",
			expiresAt: time.Now().Add(time.Hour),
		},
		{
			name:      "entry within expiry margin",
			envKeyID:  "",
			expiresAt: time.Now().Add(2 * time.Minute),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("AWS_ACCESS_KEY_ID", tt.envKeyID)

			file := cachedSessionsFile{Sessions: map[string]cachedSession{
				"work": {Profile: "work", Region: "eu-west-1", AccessKey: "AKIACACHED", ExpiresAt: tt.expiresAt},
			}}
			data, err := json.Marshal(file)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			dir := filepath.Join(home, ".lazyaws")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "session"), data, 0600); err != nil {
				t.Fatalf("write: %v", err)
			}

			if _, ok := loadCachedAWSConfig(context.Background(), "work", "eu-west-1"); ok {
				t.Errorf("loadCachedAWSConfig hit, want miss")
			}
		})
	}
}
