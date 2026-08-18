package aws

import (
	"testing"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
)

// Pure request builders lock the mutation semantics without requiring AWS.

func TestUpdateDesiredCountInput(t *testing.T) {
	got := updateDesiredCountInput("prod", "api", 3)

	if sdkaws.ToString(got.Cluster) != "prod" || sdkaws.ToString(got.Service) != "api" {
		t.Errorf("addressed %s/%s, want prod/api", sdkaws.ToString(got.Cluster), sdkaws.ToString(got.Service))
	}
	if sdkaws.ToInt32(got.DesiredCount) != 3 {
		t.Errorf("DesiredCount = %d, want 3", sdkaws.ToInt32(got.DesiredCount))
	}
	if got.ForceNewDeployment {
		t.Error("scaling also forced a new deployment")
	}
	if got.EnableExecuteCommand != nil {
		t.Error("scaling also changed the execute-command setting")
	}
}

// Zero must remain an explicit desired count because it parks a service.
func TestUpdateDesiredCountInputAcceptsZero(t *testing.T) {
	got := updateDesiredCountInput("prod", "api", 0)

	if got.DesiredCount == nil {
		t.Fatal("DesiredCount is nil for a scale-to-zero, so AWS would leave the count alone")
	}
	if sdkaws.ToInt32(got.DesiredCount) != 0 {
		t.Errorf("DesiredCount = %d, want 0", sdkaws.ToInt32(got.DesiredCount))
	}
}

func TestForceNewDeploymentInput(t *testing.T) {
	got := forceNewDeploymentInput("prod", "api")

	if !got.ForceNewDeployment {
		t.Error("ForceNewDeployment is false, so the call would be a no-op")
	}
	if got.DesiredCount != nil {
		t.Errorf("a redeploy also set DesiredCount to %d", sdkaws.ToInt32(got.DesiredCount))
	}
	if sdkaws.ToString(got.TaskDefinition) != "" {
		t.Error("a redeploy also changed the task definition; it is meant to restart on the same one")
	}
}

func TestSetExecuteCommandInput(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		got := setExecuteCommandInput("prod", "api", enabled)

		if got.EnableExecuteCommand == nil {
			t.Fatalf("EnableExecuteCommand is nil for %v, so AWS would leave it alone", enabled)
		}
		if sdkaws.ToBool(got.EnableExecuteCommand) != enabled {
			t.Errorf("EnableExecuteCommand = %v, want %v", sdkaws.ToBool(got.EnableExecuteCommand), enabled)
		}
		if got.DesiredCount != nil || got.ForceNewDeployment {
			t.Error("toggling exec also resized or redeployed the service")
		}
	}
}

// Forced deletion keeps the gated action atomic for services with running tasks.
func TestDeleteServiceInputForces(t *testing.T) {
	got := deleteServiceInput("prod", "api")

	if !sdkaws.ToBool(got.Force) {
		t.Error("Force is not set, so deleting a running service would be refused")
	}
	if sdkaws.ToString(got.Cluster) != "prod" || sdkaws.ToString(got.Service) != "api" {
		t.Errorf("addressed %s/%s, want prod/api", sdkaws.ToString(got.Cluster), sdkaws.ToString(got.Service))
	}
}

// Stop reasons distinguish operator action from a task crash in ECS events.
func TestStopTaskInputCarriesTheReason(t *testing.T) {
	const arn = "arn:aws:ecs:eu-west-1:123:task/prod/abc123"

	got := stopTaskInput("prod", arn, "stopped from lazyaws")

	if sdkaws.ToString(got.Task) != arn {
		t.Errorf("Task = %q, want the task ARN", sdkaws.ToString(got.Task))
	}
	if sdkaws.ToString(got.Cluster) != "prod" {
		t.Errorf("Cluster = %q, want prod", sdkaws.ToString(got.Cluster))
	}
	if sdkaws.ToString(got.Reason) == "" {
		t.Error("no reason recorded, so the stop reads as an unexplained exit afterwards")
	}
}
