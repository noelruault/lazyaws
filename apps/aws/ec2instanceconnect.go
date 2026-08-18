package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"
)

// SendSSHPublicKey provides a roughly 60-second connection path independent of SSM.
func (c *Client) SendSSHPublicKey(ctx context.Context, instanceID, az, osUser, publicKey string) error {
	if c.EC2InstanceConnect == nil {
		return fmt.Errorf("EC2 Instance Connect client not initialized")
	}
	if instanceID == "" {
		return fmt.Errorf("instance id is required")
	}

	_, err := c.EC2InstanceConnect.SendSSHPublicKey(ctx, &ec2instanceconnect.SendSSHPublicKeyInput{
		InstanceId:       &instanceID,
		InstanceOSUser:   &osUser,
		SSHPublicKey:     &publicKey,
		AvailabilityZone: &az,
	})
	if err != nil {
		return fmt.Errorf("failed to send SSH public key: %w", err)
	}
	return nil
}
