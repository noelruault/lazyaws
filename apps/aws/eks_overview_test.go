package aws

import (
	"context"
	"strings"
	"testing"
)

// A client with no SDK clients is the one state a test can drive without an interface seam, and it proves the fan-out's contract: every section fails, every failure is reported, and none of them takes the pane down with it.
// It is also what proves the guard is inside the fan-out: DescribeCluster, ListNodegroups and ListAddons dereference c.EKS directly, so without it this test panics in a goroutine rather than failing.
func TestGetEKSClusterOverviewReportsEverySectionThatFailed(t *testing.T) {
	overview := (&Client{}).GetEKSClusterOverview(context.Background(), "app-prod")

	if overview == nil {
		t.Fatal("GetEKSClusterOverview() = nil, want an overview even when every section failed")
	}

	for _, section := range []string{SectionCluster, SectionNodeGroups, SectionAddons} {
		err := overview.Err(section)
		if err == nil {
			t.Errorf("Err(%q) = nil, want the failed fetch to be reported", section)
			continue
		}
		if !strings.Contains(err.Error(), "EKS client not initialized") {
			t.Errorf("Err(%q) = %v, want the nil-client guard rather than a panic further in", section, err)
		}
	}

	// A failed section leaves its field zero rather than an empty value the formatter would render as an answer.
	if overview.Details != nil {
		t.Error("a failed describe should leave Details nil rather than a zero cluster the formatter would render as a described one")
	}
	if overview.NodeGroups != nil || overview.Addons != nil {
		t.Error("a failed list should stay nil rather than become an empty one the formatter would render as none")
	}
}

// Err answers per section, which is what lets one formatter section render "unavailable" while its neighbours render data.
func TestEKSOverviewErrIsPerSection(t *testing.T) {
	overview := &EKSOverview{Errs: map[string]error{SectionAddons: context.Canceled}}

	if overview.Err(SectionAddons) == nil {
		t.Error("Err() did not report the section that failed")
	}
	if overview.Err(SectionNodeGroups) != nil {
		t.Error("Err() reported a section that did not fail")
	}
}
