package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

func TestComputeUtilizationPercent(t *testing.T) {
	cases := []struct {
		name               string
		utilized, reserved float64
		want               float64
	}{
		{"half used", 512, 1024, 50},
		{"no reservation", 512, 0, 0},
		{"fully used", 1024, 1024, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := computeUtilizationPercent(tc.utilized, tc.reserved); got != tc.want {
				t.Errorf("computeUtilizationPercent(%v, %v) = %v, want %v", tc.utilized, tc.reserved, got, tc.want)
			}
		})
	}
}

func TestGetInstanceAlarmsGuards(t *testing.T) {
	if _, err := (&Client{}).GetInstanceAlarms(context.Background(), "i-1234567890"); err == nil {
		t.Error("GetInstanceAlarms() with nil CloudWatch client should error")
	}
	if _, err := (&Client{CloudWatch: nil}).GetInstanceAlarms(context.Background(), ""); err == nil {
		t.Error("GetInstanceAlarms() with empty instance id should error")
	}
}

func TestAlarmMatchesInstance(t *testing.T) {
	name, value := "InstanceId", "i-1234567890"
	other := "OtherDim"
	dims := []types.Dimension{{Name: &other, Value: &value}, {Name: &name, Value: &value}}

	if !alarmMatchesInstance(dims, "i-1234567890") {
		t.Error("expected match on InstanceId dimension")
	}
	if alarmMatchesInstance(dims, "i-0000000000") {
		t.Error("expected no match for a different instance id")
	}
	if alarmMatchesInstance(nil, "i-1234567890") {
		t.Error("expected no match with no dimensions")
	}
}
