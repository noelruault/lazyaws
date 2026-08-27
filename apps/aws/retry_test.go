package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/smithy-go"
)

// The load options are asserted by applying them, not by resolving a config: LoadDefaultConfig needs a resolvable environment and would skip exactly where this matters.
func TestBaseLoadOptionsAsksForTheAdaptiveRetryMode(t *testing.T) {
	var options awsconfig.LoadOptions
	for _, apply := range baseLoadOptions() {
		if err := apply(&options); err != nil {
			t.Fatalf("applying a load option = %v", err)
		}
	}

	if options.RetryMode != aws.RetryModeAdaptive {
		t.Errorf("LoadOptions.RetryMode = %q, want %q so a throttle slows the send rate instead of being retried into", options.RetryMode, aws.RetryModeAdaptive)
	}
}

// The cached-credentials path builds a config without ever calling LoadDefaultConfig, so a retry mode set only through load options is silently skipped on the path taken in normal operation.
// Asserted on the retryer each service client actually resolved, because the mode is only a request: it is read once, at construction, and a client built before it was assigned keeps the standard retryer.
func TestEveryClientGetsTheAdaptiveRetryerEvenWithoutLoadOptions(t *testing.T) {
	client := newClientFromConfig(aws.Config{Region: "eu-west-1"})

	// One client per SDK package that the refresh tiers drive hardest; each resolves its own options, so a shared config is not evidence that they all did.
	retryers := map[string]aws.Retryer{
		"EC2":        client.EC2.Options().Retryer,
		"EKS":        client.EKS.Options().Retryer,
		"S3":         client.S3.Options().Retryer,
		"CloudWatch": client.CloudWatch.Options().Retryer,
		"ECS":        client.ECS.Options().Retryer,
	}

	for name, retryer := range retryers {
		if _, ok := retryer.(*retry.AdaptiveMode); !ok {
			t.Errorf("%s retryer = %T, want *retry.AdaptiveMode: the standard retryer answers a throttle by retrying into it", name, retryer)
		}
	}
}

// An explicit retryer is the caller's decision and outranks the mode, which is what makes assigning the mode unconditionally safe.
func TestAnExplicitRetryerOutranksTheMode(t *testing.T) {
	standard := retry.NewStandard()
	client := newClientFromConfig(aws.Config{
		Region:  "eu-west-1",
		Retryer: func() aws.Retryer { return standard },
	})

	if _, ok := client.EC2.Options().Retryer.(*retry.AdaptiveMode); ok {
		t.Error("assigning the retry mode overrode an explicitly configured retryer")
	}
}

// The adaptive retryer is the throttle-aware one, so it has to still retry an ordinary throttling error rather than fail the call outright.
// The error is smithy's own APIError implementation rather than a hand-written stand-in: the retryer classifies on that interface, and a mock that merely looks like it (ErrorFault returning a plain int) does not satisfy it, which makes the retryable assertion pass for the wrong reason.
func TestTheAdaptiveRetryerRetriesAThrottleAndNotEverything(t *testing.T) {
	retryer := newClientFromConfig(aws.Config{Region: "eu-west-1"}).EC2.Options().Retryer

	throttled := &smithy.GenericAPIError{Code: "ThrottlingException", Message: "Rate exceeded"}
	if !retryer.IsErrorRetryable(throttled) {
		t.Error("the resolved retryer does not treat a throttling error as retryable")
	}
	if _, err := retryer.GetRetryToken(context.Background(), throttled); err != nil {
		t.Errorf("GetRetryToken() = %v, want the adaptive rate limiter to admit a first attempt", err)
	}

	// The negative case is what proves the assertion above can fail: a retryer that answered true to everything would satisfy it just as well.
	denied := &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "not authorized"}
	if retryer.IsErrorRetryable(denied) {
		t.Error("the resolved retryer retries an authorization failure, which can only fail again")
	}
}
