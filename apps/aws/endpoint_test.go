package aws

import (
	"context"
	"testing"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

// isolateAWSEnv keeps a config load from reading the operator's profile, so the only thing under test is the endpoint variable.
func isolateAWSEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_REGION", "eu-west-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "endpoint-test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "endpoint-test")
}

// The UI harness (test/ui) points the whole app at a local fake AWS with AWS_ENDPOINT_URL alone and changes no code, so that variable reaching every client is a contract this repo owes it, not an SDK detail.
// Asserted on the endpoint each service client resolved: the variable is read by LoadDefaultConfig, and a client built from a config assembled any other way silently talks to real AWS.
func TestTheEndpointURLEnvReachesEveryClient(t *testing.T) {
	isolateAWSEnv(t)
	const endpoint = "http://127.0.0.1:5055"
	t.Setenv("AWS_ENDPOINT_URL", endpoint)

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), baseLoadOptions()...)
	if err != nil {
		t.Fatalf("LoadDefaultConfig() = %v", err)
	}
	client := newClientFromConfig(cfg)

	// One client per panel that fetches, plus the identity probe the app refuses to start without.
	for name, resolved := range map[string]*string{
		"EC2":        client.EC2.Options().BaseEndpoint,
		"S3":         client.S3.Options().BaseEndpoint,
		"EKS":        client.EKS.Options().BaseEndpoint,
		"ECS":        client.ECS.Options().BaseEndpoint,
		"ECR":        client.ECR.Options().BaseEndpoint,
		"Secrets":    client.Secrets.Options().BaseEndpoint,
		"CloudWatch": client.CloudWatch.Options().BaseEndpoint,
		"STS":        client.STS.Options().BaseEndpoint,
	} {
		if resolved == nil {
			t.Errorf("%s BaseEndpoint = nil, want %q: that client would reach real AWS during a UI journey", name, endpoint)
			continue
		}
		if *resolved != endpoint {
			t.Errorf("%s BaseEndpoint = %q, want %q", name, *resolved, endpoint)
		}
	}
}

// The negative case is what proves the assertion above can fail: a client that carried some endpoint unconditionally would satisfy it just as well.
func TestWithoutTheEndpointURLEnvClientsKeepTheAWSEndpoints(t *testing.T) {
	isolateAWSEnv(t)
	t.Setenv("AWS_ENDPOINT_URL", "")

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), baseLoadOptions()...)
	if err != nil {
		t.Fatalf("LoadDefaultConfig() = %v", err)
	}

	if got := newClientFromConfig(cfg).EC2.Options().BaseEndpoint; got != nil {
		t.Errorf("EC2 BaseEndpoint = %q with no override configured, want the SDK's own resolution", *got)
	}
}
