package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
)

// ASGMembership is the ASG an EC2 instance belongs to; Desired/Min/Max describe the group's target capacity, not this one instance.
type ASGMembership struct {
	GroupName string
	Desired   int32
	Min       int32
	Max       int32
}

// GetInstanceASGMembership returns the ASG an instance belongs to, or nil (not an error) when the instance isn't part of any Auto Scaling Group.
func (c *Client) GetInstanceASGMembership(ctx context.Context, instanceID string) (*ASGMembership, error) {
	if c.AutoScaling == nil {
		return nil, fmt.Errorf("AutoScaling client not initialized")
	}
	if instanceID == "" {
		return nil, fmt.Errorf("instance id required")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 10*time.Second)
	defer cancel()

	instOut, err := c.AutoScaling.DescribeAutoScalingInstances(timeoutCtx, &autoscaling.DescribeAutoScalingInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe auto scaling instances: %w", err)
	}
	if len(instOut.AutoScalingInstances) == 0 {
		return nil, nil
	}
	groupName := getString(instOut.AutoScalingInstances[0].AutoScalingGroupName)
	if groupName == "" {
		return nil, nil
	}

	groupOut, err := c.AutoScaling.DescribeAutoScalingGroups(timeoutCtx, &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []string{groupName},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe auto scaling group: %w", err)
	}
	if len(groupOut.AutoScalingGroups) == 0 {
		return &ASGMembership{GroupName: groupName}, nil
	}

	g := groupOut.AutoScalingGroups[0]
	return &ASGMembership{
		GroupName: groupName,
		Desired:   getInt32Value(g.DesiredCapacity),
		Min:       getInt32Value(g.MinSize),
		Max:       getInt32Value(g.MaxSize),
	}, nil
}
