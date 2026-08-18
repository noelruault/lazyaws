package aws

import (
	"context"
	"testing"
)

func TestGetInstanceASGMembershipGuards(t *testing.T) {
	if _, err := (&Client{}).GetInstanceASGMembership(context.Background(), "i-1234567890"); err == nil {
		t.Error("GetInstanceASGMembership() with nil AutoScaling client should error")
	}
	if _, err := (&Client{AutoScaling: nil}).GetInstanceASGMembership(context.Background(), ""); err == nil {
		t.Error("GetInstanceASGMembership() with empty instance id should error")
	}
}
