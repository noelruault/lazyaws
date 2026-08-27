package aws

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// A client with no SDK clients is the one state a test can drive without an interface seam, and it is the state that proves the fan-out's contract: every section fails, every failure is reported, and none of them takes the pane down with it.
func TestGetInstanceOverviewReportsEverySectionThatFailed(t *testing.T) {
	overview := (&Client{}).GetInstanceOverview(context.Background(), "i-1234567890")

	if overview == nil {
		t.Fatal("GetInstanceOverview() = nil, want an overview even when every section failed")
	}

	for _, section := range []string{SectionDetails, SectionStatus, SectionMetrics} {
		if overview.Err(section) == nil {
			t.Errorf("Err(%q) = nil, want the failed fetch to be reported", section)
		}
	}

	if overview.Details != nil || overview.Status != nil || overview.Metrics != nil {
		t.Error("a failed section should leave its field nil rather than a zero value the formatter would render as data")
	}
}

// The fan-out must not fill the two sections whose cost belongs to a selection: an overview re-renders on a ticker, and DescribeAlarms pages every alarm in the account.
func TestGetInstanceOverviewLeavesTheSelectionTimeSectionsToTheCaller(t *testing.T) {
	overview := (&Client{}).GetInstanceOverview(context.Background(), "i-1234567890")

	if overview.Err(SectionASG) != nil || overview.Err(SectionAlarms) != nil {
		t.Error("GetInstanceOverview() reported an ASG or alarm error, so it fetched sections that must stay off the refresh path")
	}
	if overview.ASG != nil || overview.Alarms != nil {
		t.Error("GetInstanceOverview() filled ASG/Alarms; those are the caller's to fetch once per selection")
	}
}

// Err answers per section, which is what lets one formatter section render "unavailable" while its neighbours render data.
// Driving a genuinely partial fetch is not possible here: with a non-nil EC2 client and a real instance id the details and status calls would reach the network, so the mixed state is built rather than fetched.
func TestInstanceOverviewErrIsPerSection(t *testing.T) {
	overview := &InstanceOverview{Errs: map[string]error{SectionMetrics: context.Canceled}}

	if overview.Err(SectionMetrics) == nil {
		t.Error("Err() did not report the section that failed")
	}
	if overview.Err(SectionDetails) != nil {
		t.Error("Err() reported a section that did not fail")
	}
}

func TestGetInstanceDetailsGuards(t *testing.T) {
	_, err := (&Client{}).GetInstanceDetails(context.Background(), "i-1234567890")
	if err == nil {
		t.Fatal("GetInstanceDetails() with nil EC2 client should error")
	}
	if !strings.Contains(err.Error(), "EC2 client") {
		t.Errorf("GetInstanceDetails() nil-client error = %v, want the client guard to be what fired", err)
	}

	// A non-nil client, so only the id guard can answer: with nil the client guard fires first and hides it.
	_, err = (&Client{EC2: &ec2.Client{}}).GetInstanceDetails(context.Background(), "")
	if err == nil {
		t.Fatal("GetInstanceDetails() with empty instance id should error")
	}
	if !strings.Contains(err.Error(), "instance id required") {
		t.Errorf("GetInstanceDetails() empty-id error = %v, want the id guard to be what fired", err)
	}
}

func TestGetInstanceStatusGuards(t *testing.T) {
	_, err := (&Client{}).GetInstanceStatus(context.Background(), "i-1234567890")
	if err == nil {
		t.Fatal("GetInstanceStatus() with nil EC2 client should error")
	}
	if !strings.Contains(err.Error(), "EC2 client") {
		t.Errorf("GetInstanceStatus() nil-client error = %v, want the client guard to be what fired", err)
	}

	_, err = (&Client{EC2: &ec2.Client{}}).GetInstanceStatus(context.Background(), "")
	if err == nil {
		t.Fatal("GetInstanceStatus() with empty instance id should error")
	}
	if !strings.Contains(err.Error(), "instance id required") {
		t.Errorf("GetInstanceStatus() empty-id error = %v, want the id guard to be what fired", err)
	}
}
