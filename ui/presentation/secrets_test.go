package presentation

import (
	"testing"
	"time"

	"github.com/noelruault/lazyaws/apps/aws"
)

func TestGetSecretDisplayStrings(t *testing.T) {
	secret := &aws.SecretSummary{Name: "db-password", RotationEnabled: true}
	got := GetSecretDisplayStrings(secret)
	want := []string{"db-password", "on", "-"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("cell %d = %q, want %q", i, got[i], w)
		}
	}
}

func TestGetSecretDisplayStringsPendingDeletion(t *testing.T) {
	deleted := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	secret := &aws.SecretSummary{Name: "old-key", DeletedDate: &deleted}
	got := GetSecretDisplayStrings(secret)
	want := []string{"old-key", "off", "pending deletion"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("cell %d = %q, want %q", i, got[i], w)
		}
	}
}
