package aws

import (
	"context"
	"testing"

	aastypes "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling/types"
)

func TestGetECSServiceAutoScalingGuards(t *testing.T) {
	if _, err := (&Client{}).GetECSServiceAutoScaling(context.Background(), "prod", "web"); err == nil {
		t.Error("GetECSServiceAutoScaling() with nil ApplicationAutoScaling client should error")
	}
}

func TestToECSScalingPolicyTargetTracking(t *testing.T) {
	targetValue := 60.0
	scaleIn := int32(60)
	scaleOut := int32(30)
	metric := aastypes.MetricTypeECSServiceAverageCPUUtilization

	p := aastypes.ScalingPolicy{
		PolicyName: strPtr("cpu-target"),
		PolicyType: aastypes.PolicyTypeTargetTrackingScaling,
		TargetTrackingScalingPolicyConfiguration: &aastypes.TargetTrackingScalingPolicyConfiguration{
			TargetValue:                   &targetValue,
			ScaleInCooldown:               &scaleIn,
			ScaleOutCooldown:              &scaleOut,
			PredefinedMetricSpecification: &aastypes.PredefinedMetricSpecification{PredefinedMetricType: metric},
		},
	}

	got := toECSScalingPolicy(p)

	if got.Name != "cpu-target" || got.Type != "TargetTrackingScaling" {
		t.Fatalf("toECSScalingPolicy() name/type = %q/%q, want cpu-target/TargetTrackingScaling", got.Name, got.Type)
	}
	if got.TargetValue != 60.0 || got.TargetMetric != string(metric) {
		t.Errorf("toECSScalingPolicy() target = %v/%q, want 60/%q", got.TargetValue, got.TargetMetric, metric)
	}
	if got.ScaleInCooldownSecs != 60 || got.ScaleOutCooldownSecs != 30 {
		t.Errorf("toECSScalingPolicy() cooldowns = %d/%d, want 60/30", got.ScaleInCooldownSecs, got.ScaleOutCooldownSecs)
	}
}

func TestToECSScalingPolicyStep(t *testing.T) {
	cooldown := int32(120)
	p := aastypes.ScalingPolicy{
		PolicyName: strPtr("step-out"),
		PolicyType: aastypes.PolicyTypeStepScaling,
		StepScalingPolicyConfiguration: &aastypes.StepScalingPolicyConfiguration{
			Cooldown: &cooldown,
			StepAdjustments: []aastypes.StepAdjustment{
				{ScalingAdjustment: int32Ptr(1)},
				{ScalingAdjustment: int32Ptr(2)},
			},
		},
	}

	got := toECSScalingPolicy(p)

	if got.StepAdjustments != 2 || got.ScaleOutCooldownSecs != 120 {
		t.Errorf("toECSScalingPolicy() step = %d adjustments / %ds cooldown, want 2/120", got.StepAdjustments, got.ScaleOutCooldownSecs)
	}
}

func strPtr(s string) *string { return &s }
func int32Ptr(i int32) *int32 { return &i }
