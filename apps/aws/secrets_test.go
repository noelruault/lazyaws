package aws

import (
	"context"
	"testing"
)

func TestSecretsMutationsGuardNilClient(t *testing.T) {
	c := &Client{}
	ctx := context.Background()

	if err := c.RotateSecret(ctx, "db-password"); err == nil {
		t.Error("RotateSecret() with nil Secrets client should error")
	}
	if err := c.DeleteSecret(ctx, "db-password", 7); err == nil {
		t.Error("DeleteSecret() with nil Secrets client should error")
	}
	if err := c.RestoreSecret(ctx, "db-password"); err == nil {
		t.Error("RestoreSecret() with nil Secrets client should error")
	}
	if _, err := c.GetSecretDetails(ctx, "db-password"); err == nil {
		t.Error("GetSecretDetails() with nil Secrets client should error")
	}
	if _, err := c.ListSecrets(ctx, false); err == nil {
		t.Error("ListSecrets() with nil Secrets client should error")
	}
	if err := c.ReplicateSecretToRegions(ctx, "db-password", []string{"us-west-2"}); err == nil {
		t.Error("ReplicateSecretToRegions() with nil Secrets client should error")
	}
	if err := c.RemoveSecretReplicaRegions(ctx, "db-password", []string{"us-west-2"}); err == nil {
		t.Error("RemoveSecretReplicaRegions() with nil Secrets client should error")
	}
	if err := c.ConfigureSecretRotation(ctx, "db-password", "arn:aws:lambda:us-west-2:123456789012:function:rotate", 30); err == nil {
		t.Error("ConfigureSecretRotation() with nil Secrets client should error")
	}
}
