package aws

import (
	"context"
	"strings"
	"testing"
)

// An RDS-managed password is generated from a charset that includes & < >, and a secret's value has to be reproduced byte for byte: a displayed password that was re-encoded on the way out cannot be pasted anywhere.
func TestPrettySecretJSONPreservesTheValueVerbatim(t *testing.T) {
	got := prettySecretJSON(`{"username":"admin","password":"aB3&xY<z>Q7#pL9$mN2"}`)

	if !strings.Contains(got, `aB3&xY<z>Q7#pL9$mN2`) {
		t.Errorf("password was altered in rendering:\n%s", got)
	}
	// Re-indenting this input introduces no escapes at all, so any backslash means a character was rewritten.
	if strings.ContainsRune(got, '\\') {
		t.Errorf("value carries an escape, so copying it yields the wrong password:\n%s", got)
	}
}

// Re-encoding also sorts keys, so the rendered secret stops matching what is stored.
func TestPrettySecretJSONKeepsKeyOrder(t *testing.T) {
	got := prettySecretJSON(`{"username":"admin","password":"hunter2"}`)

	if strings.Index(got, "username") > strings.Index(got, "password") {
		t.Errorf("keys were reordered:\n%s", got)
	}
}

func TestPrettySecretJSONIgnoresNonJSON(t *testing.T) {
	if got := prettySecretJSON("just-a-plain-string"); got != "" {
		t.Errorf("a non-JSON secret has no pretty form, got %q", got)
	}
}

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
