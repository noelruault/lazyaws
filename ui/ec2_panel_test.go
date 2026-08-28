package ui

import (
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
