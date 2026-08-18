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
