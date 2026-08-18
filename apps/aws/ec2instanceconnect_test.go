package aws

import (
	"context"
	"testing"
)

func TestSendSSHPublicKeyGuards(t *testing.T) {
	if err := (&Client{}).SendSSHPublicKey(context.Background(), "i-1234567890", "us-east-1a", "ec2-user", "ssh-ed25519 AAAA"); err == nil {
		t.Error("SendSSHPublicKey() with nil EC2InstanceConnect client should error")
	}
	if err := (&Client{EC2InstanceConnect: nil}).SendSSHPublicKey(context.Background(), "", "us-east-1a", "ec2-user", "ssh-ed25519 AAAA"); err == nil {
		t.Error("SendSSHPublicKey() with empty instance id should error")
	}
}
