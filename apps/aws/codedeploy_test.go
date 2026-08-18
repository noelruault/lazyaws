package aws

import (
	"context"
	"testing"
)

func TestGetECSCodeDeployStatusGuards(t *testing.T) {
	if _, err := (&Client{}).GetECSCodeDeployStatus(context.Background(), "prod", "web"); err == nil {
		t.Error("GetECSCodeDeployStatus() with nil CodeDeploy client should error")
	}
}
