package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	aastypes "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling/types"
)

const ecsServiceNamespace = aastypes.ServiceNamespaceEcs

type ECSServiceAutoScaling struct {
	MinCapacity int32
	MaxCapacity int32
	Policies    []ECSScalingPolicy
}

// ECSScalingPolicy retains predictive scaling in Type because the UI does not expose policy-specific details for it.
type ECSScalingPolicy struct {
	Name                 string
	Type                 string // TargetTrackingScaling, StepScaling, PredictiveScaling
	TargetMetric         string // predefined metric type, or "custom metric"; empty for step policies
	TargetValue          float64
	ScaleInCooldownSecs  int32
	ScaleOutCooldownSecs int32
	StepAdjustments      int // count of step adjustments, for StepScaling policies
}

// GetECSServiceAutoScaling returns nil, nil when no scalable target is registered (not an error; most ECS services don't use Application Auto Scaling).
// Scheduled actions require a separate call and remain omitted until the UI exposes them.
func (c *Client) GetECSServiceAutoScaling(ctx context.Context, clusterName, serviceName string) (*ECSServiceAutoScaling, error) {
	if c.ApplicationAutoScaling == nil {
		return nil, fmt.Errorf("Application Auto Scaling client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()

	resourceID := fmt.Sprintf("service/%s/%s", clusterName, serviceName)

	targets, err := c.ApplicationAutoScaling.DescribeScalableTargets(timeoutCtx, &applicationautoscaling.DescribeScalableTargetsInput{
		ServiceNamespace: ecsServiceNamespace,
		ResourceIds:      []string{resourceID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe scalable targets: %w", err)
	}
	if len(targets.ScalableTargets) == 0 {
		return nil, nil
	}
	target := targets.ScalableTargets[0]

	policiesOut, err := c.ApplicationAutoScaling.DescribeScalingPolicies(timeoutCtx, &applicationautoscaling.DescribeScalingPoliciesInput{
		ServiceNamespace: ecsServiceNamespace,
		ResourceId:       &resourceID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe scaling policies: %w", err)
	}

	result := &ECSServiceAutoScaling{
		MinCapacity: getInt32Value(target.MinCapacity),
		MaxCapacity: getInt32Value(target.MaxCapacity),
	}
	for _, p := range policiesOut.ScalingPolicies {
		result.Policies = append(result.Policies, toECSScalingPolicy(p))
	}
	return result, nil
}

func toECSScalingPolicy(p aastypes.ScalingPolicy) ECSScalingPolicy {
	policy := ECSScalingPolicy{
		Name: getString(p.PolicyName),
		Type: string(p.PolicyType),
	}
	if cfg := p.TargetTrackingScalingPolicyConfiguration; cfg != nil {
		policy.TargetValue = getFloat64Value(cfg.TargetValue)
		policy.ScaleInCooldownSecs = getInt32Value(cfg.ScaleInCooldown)
		policy.ScaleOutCooldownSecs = getInt32Value(cfg.ScaleOutCooldown)
		switch {
		case cfg.PredefinedMetricSpecification != nil:
			policy.TargetMetric = string(cfg.PredefinedMetricSpecification.PredefinedMetricType)
		case cfg.CustomizedMetricSpecification != nil:
			policy.TargetMetric = "custom metric"
		}
	}
	if cfg := p.StepScalingPolicyConfiguration; cfg != nil {
		policy.ScaleOutCooldownSecs = getInt32Value(cfg.Cooldown)
		policy.StepAdjustments = len(cfg.StepAdjustments)
	}
	return policy
}

func getFloat64Value(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
