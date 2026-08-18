package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
)

// ECSTargetHealth remains distinct because target-group health can disagree with ECS task state.
type ECSTargetHealth struct {
	TargetID string
	Port     int32
	State    string // healthy, unhealthy, draining, initial, unavailable, unused
	Reason   string
}

func (c *Client) DescribeTargetHealth(ctx context.Context, targetGroupArn string) ([]ECSTargetHealth, error) {
	if c.ELBv2 == nil {
		return nil, fmt.Errorf("ELBv2 client not initialized")
	}
	if targetGroupArn == "" {
		return nil, fmt.Errorf("target group ARN required")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 10*time.Second)
	defer cancel()

	out, err := c.ELBv2.DescribeTargetHealth(timeoutCtx, &elasticloadbalancingv2.DescribeTargetHealthInput{
		TargetGroupArn: &targetGroupArn,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe target health: %w", err)
	}

	health := make([]ECSTargetHealth, 0, len(out.TargetHealthDescriptions))
	for _, d := range out.TargetHealthDescriptions {
		th := ECSTargetHealth{}
		if d.Target != nil {
			th.TargetID = getString(d.Target.Id)
			th.Port = getInt32Value(d.Target.Port)
		}
		if d.TargetHealth != nil {
			th.State = string(d.TargetHealth.State)
			th.Reason = string(d.TargetHealth.Reason)
		}
		health = append(health, th)
	}
	return health, nil
}
