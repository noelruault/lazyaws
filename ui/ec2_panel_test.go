package ui

import (
	"strings"
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
)

func TestFormatByteCount(t *testing.T) {
	cases := []struct {
		bytes float64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1024 * 1024, "1.0 MiB"},
	}
	for _, tc := range cases {
		if got := formatByteCount(tc.bytes); got != tc.want {
			t.Errorf("formatByteCount(%v) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

func TestFormatEC2StorageEmpty(t *testing.T) {
	if got := formatEC2Storage(nil, nil); got != "no EBS volumes\n" {
		t.Errorf("formatEC2Storage(nil, nil) = %q, want no-volumes message", got)
	}
}

func TestFormatEC2StorageSnapshots(t *testing.T) {
	devices := []aws.BlockDevice{{DeviceName: "/dev/xvda", VolumeID: "vol-1", VolumeSize: 8, VolumeType: "gp3"}}

	out := formatEC2Storage(devices, nil)
	if !strings.Contains(out, "Snapshots:\nnone") {
		t.Errorf("formatEC2Storage with no snapshots should report none, got: %q", out)
	}

	snapshots := []aws.VolumeSnapshot{{SnapshotID: "snap-1", VolumeID: "vol-1", State: "completed", Progress: "100%", SizeGiB: 8, StartTime: "2026-07-10 00:00:00"}}
	out = formatEC2Storage(devices, snapshots)
	if !strings.Contains(out, "snap-1 vol:vol-1 completed 100% (8 GiB)") {
		t.Errorf("formatEC2Storage should list the snapshot, got: %q", out)
	}
}

func TestFormatEC2SecurityEmpty(t *testing.T) {
	if got := formatEC2Security(nil); got != "no security groups\n" {
		t.Errorf("formatEC2Security(nil) = %q, want no-groups message", got)
	}
}

func TestFormatEC2ConfigNilASG(t *testing.T) {
	d := &aws.InstanceDetails{Instance: aws.Instance{ID: "i-123", State: "running"}}
	out := formatEC2Config(d, nil)
	if !strings.Contains(out, "Auto Scaling Group:\nnone") {
		t.Errorf("formatEC2Config with nil ASG should report none, got: %q", out)
	}
}

// The EC2 panel sorts running instances first, so a reload that stops one instance reorders the list under the cursor.
// Selecting by index there would leave the detail pane describing a different instance than the highlighted row.
func TestEC2ReloadKeepsTheSelectedInstance(t *testing.T) {
	gui := newTestGui(t)

	web := &aws.Instance{ID: "i-web", Name: "web", State: "running"}
	worker := &aws.Instance{ID: "i-worker", Name: "worker", State: "running"}
	batch := &aws.Instance{ID: "i-batch", Name: "batch", State: "stopped"}

	gui.Panels.EC2.SetItems([]*aws.Instance{web, worker, batch})
	if !gui.Panels.EC2.SelectByItem(worker) {
		t.Fatal("SelectByItem(worker) = false, want the row the test is about")
	}

	// A reload hands back fresh pointers, which is why identity has to be the instance id and not the item itself.
	reloaded := []*aws.Instance{
		{ID: "i-batch", Name: "batch", State: "running"},
		{ID: "i-worker", Name: "worker", State: "stopped"},
		{ID: "i-web", Name: "web", State: "running"},
	}
	gui.Panels.EC2.SetItemsKeepSelection(reloaded, func(inst *aws.Instance) string { return inst.ID })

	selected, err := gui.Panels.EC2.GetSelectedItem()
	if err != nil {
		t.Fatalf("GetSelectedItem() error = %v", err)
	}
	if selected.ID != "i-worker" {
		t.Errorf("selected = %s, want i-worker (the row that was selected before the reload)", selected.ID)
	}
	// The stopped worker sorts last now, so the fix is only real if the index moved with it.
	if got := gui.Panels.EC2.SelectedIdx; got != 2 {
		t.Errorf("SelectedIdx = %d, want 2 (running-first sort moves the stopped worker to the end)", got)
	}
}
