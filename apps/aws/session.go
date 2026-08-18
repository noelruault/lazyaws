package aws

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

type cachedSession struct {
	Profile     string    `json:"profile"`
	Region      string    `json:"region"`
	AccessKey   string    `json:"access_key"`
	SecretKey   string    `json:"secret_key"`
	SessionTok  string    `json:"session_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	LastUpdated time.Time `json:"last_updated"`
}

type cachedSessionsFile struct {
	Sessions map[string]cachedSession `json:"sessions"`
}

func loadCachedAWSConfig(ctx context.Context, profile, region string) (aws.Config, bool) {
	// Explicit env credentials keep standard-chain precedence over the cache.
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" {
		return aws.Config{}, false
	}
	entry, err := loadCachedSession(profile)
	if err != nil || entry == nil {
		return aws.Config{}, false
	}
	// Reject entries expiring within a 5 minute margin, not just already-expired ones.
	if time.Now().Add(5 * time.Minute).After(entry.ExpiresAt) {
		return aws.Config{}, false
	}
	if region == "" {
		region = entry.Region
	}
	if region == "" {
		return aws.Config{}, false
	}
	// Static cached credentials cannot refresh, so the expiry margin and identity probe bound stale reuse.
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			entry.AccessKey, entry.SecretKey, entry.SessionTok,
		)),
	)
	if err != nil {
		return aws.Config{}, false
	}
	return cfg, true
}

func saveCachedCredentials(ctx context.Context, profile string, cfg aws.Config) error {
	if profile == "" {
		profile = "default"
	}

	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return err
	}

	// Only genuinely temporary credentials are worth persisting; never synthesize an expiry.
	if !creds.CanExpire || creds.Expires.IsZero() {
		return nil
	}

	entry := cachedSession{
		Profile:     profile,
		Region:      cfg.Region,
		AccessKey:   creds.AccessKeyID,
		SecretKey:   creds.SecretAccessKey,
		SessionTok:  creds.SessionToken,
		ExpiresAt:   creds.Expires,
		LastUpdated: time.Now(),
	}

	cachePath, err := sessionCachePath()
	if err != nil {
		return err
	}

	fileData := cachedSessionsFile{Sessions: map[string]cachedSession{}}
	if data, err := os.ReadFile(cachePath); err == nil {
		_ = json.Unmarshal(data, &fileData)
	}
	if fileData.Sessions == nil {
		fileData.Sessions = map[string]cachedSession{}
	}
	fileData.Sessions[profile] = entry

	data, err := json.MarshalIndent(fileData, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cachePath, data, 0600)
}

func loadCachedSession(profile string) (*cachedSession, error) {
	if profile == "" {
		profile = "default"
	}
	cachePath, err := sessionCachePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}
	var file cachedSessionsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	if entry, ok := file.Sessions[profile]; ok {
		return &entry, nil
	}
	return nil, os.ErrNotExist
}

func sessionCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".lazyaws")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}

	path := filepath.Join(dir, "session")
	// Legacy cache names are migrated once so upgrades preserve active sessions.
	for _, legacy := range []string{"session-cache.json", "session.json"} {
		legacyPath := filepath.Join(dir, legacy)
		if _, err := os.Stat(legacyPath); err == nil {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				_ = os.Rename(legacyPath, path)
			} else {
				_ = os.Remove(legacyPath)
			}
		}
	}
	return path, nil
}

func currentProfile() string {
	if p := os.Getenv("AWS_PROFILE"); p != "" {
		return p
	}
	return "default"
}
