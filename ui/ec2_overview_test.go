package ui

import (
	"context"
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
)

// A client with no SDK clients fails both extra fetches, which is what makes the memo observable without an interface seam: a refetch would overwrite whatever the cache is holding.
func newExtrasOverview() *aws.InstanceOverview {
	return &aws.InstanceOverview{Errs: map[string]error{}}
}

// The alarms and ASG lookups are fetched once per selected instance, because the overview re-renders on a ticker and DescribeAlarms pages every alarm in the account.
// A sentinel written into the cache after the first fetch is what proves the second call read the cache rather than calling out again.
func TestEC2OverviewExtrasAreFetchedOncePerSelection(t *testing.T) {
	var extras ec2OverviewExtras
	client := &aws.Client{}

	first := newExtrasOverview()
	extras.fill(context.Background(), client, 1, "i-1", first)
	if first.Err(aws.SectionAlarms) == nil || first.Err(aws.SectionASG) == nil {
		t.Fatal("fill() did not report the failed extra fetches onto the overview")
	}

	sentinel := []aws.InstanceAlarm{{Name: "held-from-the-first-fetch"}}
	extras.alarms = sentinel

	second := newExtrasOverview()
	extras.fill(context.Background(), client, 1, "i-1", second)
	if len(second.Alarms) != 1 || second.Alarms[0].Name != sentinel[0].Name {
		t.Errorf("a second render of the same instance refetched the alarms; alarms = %v", second.Alarms)
	}
}

func TestEC2OverviewExtrasRefetchWhenTheSelectionOrProfileMoves(t *testing.T) {
	tests := []struct {
		name       string
		gen        int
		instanceID string
	}{
		{name: "another instance", gen: 1, instanceID: "i-2"},
		// An instance id is only unique within the account it was read from, and a profile switch replaces the account without changing the id.
		{name: "another profile", gen: 2, instanceID: "i-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var extras ec2OverviewExtras
			client := &aws.Client{}

			extras.fill(context.Background(), client, 1, "i-1", newExtrasOverview())
			extras.alarms = []aws.InstanceAlarm{{Name: "held-from-the-first-fetch"}}

			got := newExtrasOverview()
			extras.fill(context.Background(), client, tt.gen, tt.instanceID, got)
			if len(got.Alarms) != 0 {
				t.Errorf("%s reused the previous selection's alarms: %v", tt.name, got.Alarms)
			}
		})
	}
}

// The overview must not report an ASG failure on an instance that simply belongs to no group: nil with no error is how that is reported, and reading it as a failure would claim a broken permission on the commonest case there is.
func TestEC2OverviewExtrasCopyErrorsWithoutInventingThem(t *testing.T) {
	var extras ec2OverviewExtras
	extras.instanceID, extras.gen = "i-1", 1
	extras.errs = map[string]error{}

	overview := newExtrasOverview()
	extras.fill(context.Background(), &aws.Client{}, 1, "i-1", overview)

	if overview.Err(aws.SectionASG) != nil || overview.Err(aws.SectionAlarms) != nil {
		t.Errorf("fill() invented an error for a section that succeeded: %v", overview.Errs)
	}
	if overview.ASG != nil {
		t.Error("fill() should leave a non-member instance's ASG nil")
	}
}

// The snapshots need the volume ids off the ticker's details fetch, so a render where details failed must not latch an empty snapshot list for the whole selection.
func TestEC2OverviewExtrasSnapshotsWaitForTheDetails(t *testing.T) {
	var extras ec2OverviewExtras

	// First render: details failed, so the snapshot fetch must not run or latch.
	withoutDetails := newExtrasOverview()
	extras.fill(context.Background(), &aws.Client{}, 1, "i-1", withoutDetails)
	if extras.snapsFilled {
		t.Fatal("fill() latched the snapshot list before the volume ids were known")
	}

	// A later render of the same selection has the details; the fetch runs (and fails on the empty client), which is the latch plus a reported error.
	withDetails := newExtrasOverview()
	withDetails.Details = &aws.InstanceDetails{BlockDevices: []aws.BlockDevice{{VolumeID: "vol-1"}}}
	extras.fill(context.Background(), &aws.Client{}, 1, "i-1", withDetails)
	if !extras.snapsFilled {
		t.Fatal("fill() did not fetch the snapshots once the details arrived")
	}
	if withDetails.Err(aws.SectionSnapshots) == nil {
		t.Error("a failed snapshot fetch should be reported on the overview")
	}
}
