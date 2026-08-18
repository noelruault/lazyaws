package ui

import (
	"testing"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
)

func newTestGui(t *testing.T) *Gui {
	t.Helper()

	gui := &Gui{
		Views: Views{
			Profile: &gocui.View{},
			ECS:     &gocui.View{},
			EC2:     &gocui.View{},
			S3:      &gocui.View{},
			EKS:     &gocui.View{},
			ECR:     &gocui.View{},
			Secrets: &gocui.View{},

			VPC: &gocui.View{},

			Menu: &gocui.View{},
			Main: &gocui.View{},
		},
		State:            guiState{Panels: &panelStates{Main: &mainPanelState{}}},
		throttledRefresh: newThrottle(time.Millisecond, func() {}),
	}
	gui.setPanels()
	return gui
}

// Profile switches must clear every resource-specific item and selection.
func TestProfileSwitchResetsState(t *testing.T) {
	gui := newTestGui(t)

	gui.Panels.ECS.SetItems([]*ecsRow{{Kind: ecsRowKindCluster, Cluster: &aws.ECSCluster{Name: "c1"}}})
	gui.ecsDrill = ecsDrillState{level: ecsLevelServices, cluster: "c1"}
	gui.Views.ECS.Title = "ECS: c1"

	gui.Panels.EC2.SetItems([]*aws.Instance{{ID: "i-1", Name: "web-1"}})
	gui.Panels.S3.SetItems([]*aws.Bucket{{Name: "b1"}})
	gui.s3Objects = s3ObjectsState{bucket: "b1", prefix: "logs/", objects: []aws.S3Object{{Key: "logs/a"}}}
	gui.Panels.EKS.SetItems([]*aws.EKSCluster{{Name: "k1"}})
	gui.Panels.ECR.SetItems([]*aws.ECRRepository{{Name: "r1"}})
	gui.Panels.Secrets.SetItems([]*aws.SecretSummary{{Name: "s1"}})
	gui.secretsReveal = secretsRevealState{lastItem: "s1", revealed: "s1"}
	gui.secretsShowDeleted = true
	gui.State.Panels.Main.ObjectKey = "ec2-i-1"

	gui.resetDependentPanelState()

	if got := gui.Panels.ECS.List.GetItems(); len(got) != 0 {
		t.Errorf("ECS items not reset: %v", got)
	}
	if gui.ecsDrill != (ecsDrillState{}) {
		t.Errorf("ecsDrill not reset: %+v", gui.ecsDrill)
	}
	if gui.Views.ECS.Title != "ECS" {
		t.Errorf("ECS view title not reset: %q", gui.Views.ECS.Title)
	}
	if got := gui.Panels.EC2.List.GetItems(); len(got) != 0 {
		t.Errorf("EC2 items not reset: %v", got)
	}
	if got := gui.Panels.S3.List.GetItems(); len(got) != 0 {
		t.Errorf("S3 items not reset: %v", got)
	}
	if gui.s3Objects.bucket != "" || gui.s3Objects.prefix != "" || gui.s3Objects.objects != nil {
		t.Errorf("s3Objects not reset: %+v", gui.s3Objects)
	}
	if got := gui.Panels.EKS.List.GetItems(); len(got) != 0 {
		t.Errorf("EKS items not reset: %v", got)
	}
	if got := gui.Panels.ECR.List.GetItems(); len(got) != 0 {
		t.Errorf("ECR items not reset: %v", got)
	}
	if got := gui.Panels.Secrets.List.GetItems(); len(got) != 0 {
		t.Errorf("Secrets items not reset: %v", got)
	}
	if gui.secretsReveal != (secretsRevealState{}) {
		t.Errorf("secretsReveal not reset: %+v", gui.secretsReveal)
	}
	if gui.secretsShowDeleted {
		t.Error("secretsShowDeleted not reset")
	}
	if gui.State.Panels.Main.ObjectKey != "" {
		t.Errorf("main ObjectKey not reset: %q", gui.State.Panels.Main.ObjectKey)
	}
}

// A stale connection must never replace a newer profile switch.
func TestStaleGenerationMsgsDropped(t *testing.T) {
	gui := newTestGui(t)

	gui.Gen = 1
	staleGen := gui.Gen
	staleClient := &aws.Client{}

	gui.Gen = 2
	gui.CurrentProfile = "newer"

	if err := gui.applyProfileSwitch(staleGen, "stale", staleClient); err != nil {
		t.Fatalf("applyProfileSwitch() error = %v", err)
	}

	if gui.CurrentProfile != "newer" {
		t.Errorf("CurrentProfile = %q, want %q (stale switch must not overwrite it)", gui.CurrentProfile, "newer")
	}
	if gui.Client == staleClient {
		t.Error("stale client must not be installed")
	}

	gen := gui.Gen
	client := &aws.Client{}
	if err := gui.applyProfileSwitch(gen, "current", client); err != nil {
		t.Fatalf("applyProfileSwitch() error = %v", err)
	}
	if gui.CurrentProfile != "current" {
		t.Errorf("CurrentProfile = %q, want %q", gui.CurrentProfile, "current")
	}
	if gui.Client != client {
		t.Error("current-gen client was not installed")
	}
}
