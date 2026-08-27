package aws

import (
	"context"
	"testing"
)

// Both sections fetch through a concrete SDK client, so a nil client is the one way a test can observe WHICH fetches were attempted and that each failure stays on its own key.
func TestGetECSServiceOverviewReportsEachSectionSeparately(t *testing.T) {
	overview := (&Client{}).GetECSServiceOverview(context.Background(), &ECSService{
		Cluster: "app-cluster",
		Name:    "app-auth",
	})

	if overview == nil {
		t.Fatal("GetECSServiceOverview must always return an overview, never nil")
	}
	for _, section := range []string{SectionMetrics, SectionImage} {
		if overview.Err(section) == nil {
			t.Errorf("Err(%q) = nil, want the nil-client failure reported against its own section", section)
		}
	}
	if overview.Metrics != nil || overview.Image.Image != "" {
		t.Error("a failed fetch must leave its field empty rather than half-filled")
	}
}

// The pane renders from the selected row, which can be nil while a panel is still loading; a fan-out that dereferenced it would take the whole app down rather than one tab.
func TestGetECSServiceOverviewWithoutAServiceAttemptsNothing(t *testing.T) {
	overview := (&Client{}).GetECSServiceOverview(context.Background(), nil)

	if len(overview.Errs) != 0 {
		t.Errorf("Errs = %v, want no fetch attempted without a service", overview.Errs)
	}
}
