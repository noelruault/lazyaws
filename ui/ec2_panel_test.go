package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

func TestFormatEC2MetricsStampsReadingsAndKeepsAbsenceAbsent(t *testing.T) {
	at := time.Date(2026, 8, 27, 17, 43, 0, 0, time.UTC)
	metrics := &aws.InstanceMetrics{
		InstanceID:     "i-0abcdef1234567890",
		CPUUtilization: aws.MetricPoint{Value: 0.523, At: at, OK: true},
		NetworkIn:      aws.MetricPoint{Value: 235000, At: at, OK: true},
		// An EBS-only instance publishes no disk metrics at all.
		DiskReadBytes:     aws.MetricPoint{},
		DiskWriteBytes:    aws.MetricPoint{},
		StatusCheckFailed: aws.MetricPoint{Value: 0, At: at, OK: true},
	}

	got := utils.Decolorise(formatEC2Metrics(metrics))

	for _, want := range []string{
		"CPU utilization: 0.5% (5-min avg @ 17:43Z)",
		"Network in: 229.5 KiB (5-min total @ 17:43Z)",
		"Disk read: no data",
		"Disk write: no data",
		"Status check failed: 0 (5-min max @ 17:43Z)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("formatEC2Metrics() missing %q, got:\n%s", want, got)
		}
	}

	// The old caption claimed a freshness basic monitoring cannot deliver: its datapoints are already minutes old on arrival.
	if strings.Contains(strings.ToLower(got), "last 5 minutes") {
		t.Errorf("formatEC2Metrics() still captions a fixed period, got:\n%s", got)
	}
	// A metric that published nothing must never be rendered as a zero reading.
	if strings.Contains(got, "Disk read: 0") {
		t.Errorf("formatEC2Metrics() rendered an absent series as 0, got:\n%s", got)
	}
}

func TestFormatMetricPointAbsentReadsNoData(t *testing.T) {
	got := formatMetricPoint(aws.MetricPoint{Value: 99, At: time.Now(), OK: false}, "5-min avg", func(v float64) string {
		return "formatted"
	})
	if got != "no data" {
		t.Errorf("formatMetricPoint() = %q, want %q; an absent point must not render its zero value", got, "no data")
	}
}

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
	// The selected instance carries no Name tag, so only its id can identify it: a panel keyed on the name would have nothing to match.
	unnamed := &aws.Instance{ID: "i-worker", State: "running"}
	batch := &aws.Instance{ID: "i-batch", Name: "batch", State: "stopped"}

	gui.Panels.EC2.SetItems([]*aws.Instance{web, unnamed, batch})
	if !gui.Panels.EC2.SelectByItem(unnamed) {
		t.Fatal("SelectByItem(unnamed) = false, want the row the test is about")
	}

	// A reload hands back fresh pointers, which is why identity has to be the instance id and not the item itself.
	reloaded := []*aws.Instance{
		{ID: "i-batch", Name: "batch", State: "running"},
		{ID: "i-worker", State: "stopped"},
		{ID: "i-web", Name: "web", State: "running"},
	}
	gui.Panels.EC2.SetItemsKeepSelection(reloaded, ec2SelectionKey)

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

// Each panel's reload key has to be the field that identifies the resource, not the one it happens to show first.
// A key copied from a neighbouring panel still compiles and still keeps a selection, so nothing but this table notices.
func TestSelectionKeysAreTheResourceIdentity(t *testing.T) {
	for _, tt := range []struct {
		panel string
		got   string
		want  string
	}{
		{"ec2", ec2SelectionKey(&aws.Instance{ID: "i-1", Name: "web", State: "running"}), "i-1"},
		{"ec2 without a name tag", ec2SelectionKey(&aws.Instance{ID: "i-2"}), "i-2"},
		{"s3", s3SelectionKey(&aws.Bucket{Name: "logs"}), "logs"},
		{"eks", eksSelectionKey(&aws.EKSCluster{Name: "prod", Version: "1.29"}), "prod"},
		{"ecr", ecrSelectionKey(&aws.ECRRepository{Name: "svc-api"}), "svc-api"},
		{"secrets", secretsSelectionKey(&aws.SecretSummary{Name: "db-password"}), "db-password"},
		{"vpc", vpcSelectionKey(&aws.VPC{ID: "vpc-1", CIDR: "10.0.0.0/16"}), "vpc-1"},
		{"profile", profileSelectionKey("staging"), "staging"},
		{"ecs cluster", ecsSelectionKey(&ecsRow{Kind: ecsRowKindCluster, Cluster: &aws.ECSCluster{Name: "c1", Arn: "arn:cluster"}}), "arn:cluster"},
		{"ecs service", ecsSelectionKey(&ecsRow{Kind: ecsRowKindService, Service: &aws.ECSService{Name: "svc", Arn: "arn:service"}}), "arn:service"},
		{"ecs task", ecsSelectionKey(&ecsRow{Kind: ecsRowKindTask, Task: &aws.ECSTask{ID: "t1", Arn: "arn:task"}}), "arn:task"},
	} {
		if tt.got != tt.want {
			t.Errorf("%s selection key = %q, want %q", tt.panel, tt.got, tt.want)
		}
	}
}
