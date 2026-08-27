package aws

import (
	"context"
	"testing"
)

// Every section here fetches through a concrete SDK client, so a nil client is the one way a test can observe WHICH sections were attempted.
// That is what this pins: the metrics call is skipped on a cluster with Insights off, and skipping it is recorded rather than left looking like a fetch that came back empty.
func TestGetECSClusterOverviewSkipsMetricsWhenInsightsIsOff(t *testing.T) {
	for _, tc := range []struct {
		name            string
		insights        string
		wantInsightsOff bool
	}{
		{"never listed", "", true},
		{"explicitly disabled", "disabled", true},
		{"enabled", "enabled", false},
		{"enhanced is a tier above enabled, not a different answer", "enhanced", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			overview := (&Client{}).GetECSClusterOverview(context.Background(), &ECSCluster{
				Name:              "app-cluster",
				ContainerInsights: tc.insights,
			})

			if overview.InsightsOff != tc.wantInsightsOff {
				t.Errorf("InsightsOff = %v, want %v", overview.InsightsOff, tc.wantInsightsOff)
			}

			// With a nil client every attempted fetch fails, so an error present means the call was made and an error absent means it never was.
			_, attempted := overview.Errs[SectionMetrics]
			if attempted == tc.wantInsightsOff {
				t.Errorf("metrics attempted = %v with insights %q; a cluster without Insights would be billed for four empty series on every refresh tick", attempted, tc.insights)
			}
		})
	}
}

// One failed section must cost its own block and not the pane, which is the whole point of the fan-out.
func TestGetECSClusterOverviewReportsEachSectionSeparately(t *testing.T) {
	overview := (&Client{}).GetECSClusterOverview(context.Background(), &ECSCluster{
		Name:              "app-cluster",
		ContainerInsights: "enabled",
	})

	if overview == nil {
		t.Fatal("GetECSClusterOverview must always return an overview, never nil")
	}
	for _, section := range []string{SectionServices, SectionTasks, SectionMetrics} {
		if overview.Err(section) == nil {
			t.Errorf("Err(%q) = nil, want the nil-client failure reported against its own section", section)
		}
	}
	if overview.Services != nil || overview.Tasks != nil || overview.Metrics != nil {
		t.Error("a failed fetch must leave its field empty rather than half-filled")
	}
}
