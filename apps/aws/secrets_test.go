package aws

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	secretsmanagertypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// The empty policy is both states, so a read that failed has to leave the error behind: dropping it is what made the pane call an unreadable policy an absent one, and the drop is invisible from the field alone.
func TestResourcePolicyResultTellsAFailedReadFromAnAbsentPolicy(t *testing.T) {
	readErr := errors.New("AccessDenied")

	for _, tt := range []struct {
		name    string
		out     *secretsmanager.GetResourcePolicyOutput
		err     error
		want    string
		wantErr error
	}{
		{name: "the read failed", err: readErr, wantErr: readErr},
		{name: "a policy that failed to read is never kept", out: &secretsmanager.GetResourcePolicyOutput{ResourcePolicy: aws.String(`{"Version":"2012-10-17"}`)}, err: readErr, wantErr: readErr},
		{name: "no policy attached", out: &secretsmanager.GetResourcePolicyOutput{}},
		{name: "a policy is attached", out: &secretsmanager.GetResourcePolicyOutput{ResourcePolicy: aws.String(`{"Version":"2012-10-17"}`)}, want: `{"Version":"2012-10-17"}`},
	} {
		policy, err := resourcePolicyResult(tt.out, tt.err)
		if policy != tt.want || !errors.Is(err, tt.wantErr) {
			t.Errorf("resourcePolicyResult(%s) = (%q, %v), want (%q, %v)", tt.name, policy, err, tt.want, tt.wantErr)
		}
	}
}

// RotationRules is absent on every secret that has never had rotation configured, and AutomaticallyAfterDays is absent again on one scheduled by a cron() or rate() expression that has not rotated yet.
func TestRotationDaysIsNilSafe(t *testing.T) {
	if got := rotationDays(nil); got != 0 {
		t.Errorf("rotationDays(nil) = %d, want 0", got)
	}
	if got := rotationDays(&secretsmanagertypes.RotationRulesType{}); got != 0 {
		t.Errorf("rotationDays with no cadence = %d, want 0", got)
	}

	days := int64(7)
	if got := rotationDays(&secretsmanagertypes.RotationRulesType{AutomaticallyAfterDays: &days}); got != 7 {
		t.Errorf("rotationDays = %d, want 7", got)
	}
}

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
