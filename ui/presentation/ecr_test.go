package presentation

import (
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
)

func TestGetECRRepositoryDisplayStrings(t *testing.T) {
	repo := &aws.ECRRepository{Name: "svc-api", TagMutability: "IMMUTABLE", ScanOnPush: true}
	got := GetECRRepositoryDisplayStrings(repo)
	want := []string{"svc-api", "IMMUTABLE", "on"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("cell %d = %q, want %q", i, got[i], w)
		}
	}
}

func TestGetECRRepositoryDisplayStringsDefaults(t *testing.T) {
	repo := &aws.ECRRepository{Name: "svc-worker"}
	got := GetECRRepositoryDisplayStrings(repo)
	want := []string{"svc-worker", "-", "off"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("cell %d = %q, want %q", i, got[i], w)
		}
	}
}
