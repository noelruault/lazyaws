package aws

import (
	"context"
	"testing"
)

func TestDescribeTargetHealthGuards(t *testing.T) {
	if _, err := (&Client{}).DescribeTargetHealth(context.Background(), "arn:aws:elasticloadbalancing:tg/web"); err == nil {
		t.Error("DescribeTargetHealth() with nil ELBv2 client should error")
	}
	if _, err := (&Client{ELBv2: nil}).DescribeTargetHealth(context.Background(), ""); err == nil {
		t.Error("DescribeTargetHealth() with empty ARN should error")
	}
}
