package aws

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/codedeploy"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/logging"
)

type Config struct {
	Region string
}

type Client struct {
	EC2                    *ec2.Client
	EC2InstanceConnect     *ec2instanceconnect.Client
	S3                     *s3.Client
	EKS                    *eks.Client
	ECS                    *ecs.Client
	ECR                    *ecr.Client
	Secrets                *secretsmanager.Client
	SSM                    *ssm.Client
	CloudWatch             *cloudwatch.Client
	CloudWatchLogs         *cloudwatchlogs.Client
	ELBv2                  *elasticloadbalancingv2.Client
	CodeDeploy             *codedeploy.Client
	ApplicationAutoScaling *applicationautoscaling.Client
	AutoScaling            *autoscaling.Client
	STS                    *sts.Client
	BedrockRuntime         *bedrockruntime.Client
	Bedrock                *bedrock.Client
	// chatModelIDs caches configured-model to invocable-id resolutions (see resolveChatModel), so a question doesn't re-list inference profiles every time.
	chatModelsMu sync.Mutex
	chatModelIDs map[string]string
	// instanceTypes caches DescribeInstanceTypes answers (see GetInstanceTypeInfo): a type's vCPU, memory and network performance are properties of the type, not of any instance, so re-asking can only return what is already held.
	instanceTypesMu sync.Mutex
	instanceTypes   map[string]InstanceTypeInfo
	// clusterInsights records each cluster's containerInsights setting as the last cluster list read it, so a service metrics fetch can decide whether the Insights namespace is worth querying without a describe of its own.
	// It is a record, not a cache: nothing reads it to skip an API call, so a setting changed since the last list costs at most one refresh of an additive extra.
	clusterInsightsMu sync.Mutex
	clusterInsights   map[string]string
	// taskDefs caches task definition revisions (see DescribeTaskDefinitionDetail), which are immutable once registered, so the overview refresh tier does not re-describe the same revision every tick.
	taskDefsMu sync.Mutex
	taskDefs   map[string]*ECSTaskDefinitionDetail
	// The metrics tier's memos, one per pane that reads CloudWatch (see metricsMemo): an overview redraws every couple of seconds and its metrics refetch on their own, much slower interval, because GetMetricData is the one call here that is billed per metric requested.
	instanceMetrics metricsMemo[*InstanceMetrics]
	clusterMetrics  metricsMemo[*ECSClusterMetrics]
	serviceMetrics  metricsMemo[*ECSServiceMetrics]
	// identityErr is why the STS caller-identity probe last failed, kept so the bootstrap can refuse to start rather than let every panel discover the same expired token on its own.
	identityErr error
	Region      string
	AccountID   string
}

func newClientFromConfig(cfg aws.Config) *Client {
	// Every client is built here, including the cached-credentials path that never calls LoadDefaultConfig, so this is the only place the SDK logger and the retry mode cannot be bypassed.
	cfg.Logger = sdkLogger{}
	// A retry mode set only through load options is skipped on the cached path, which is the path taken in normal operation.
	// Each service client resolves cfg.RetryMode into its own retryer as it is constructed, so this has to be assigned before the clients below are built. An explicit cfg.Retryer still wins: the SDK resolves that first and leaves the mode unread.
	cfg.RetryMode = retryMode

	return &Client{
		EC2:                    ec2.NewFromConfig(cfg),
		EC2InstanceConnect:     ec2instanceconnect.NewFromConfig(cfg),
		S3:                     s3.NewFromConfig(cfg),
		EKS:                    eks.NewFromConfig(cfg),
		ECS:                    ecs.NewFromConfig(cfg),
		ECR:                    ecr.NewFromConfig(cfg),
		Secrets:                secretsmanager.NewFromConfig(cfg),
		SSM:                    ssm.NewFromConfig(cfg),
		CloudWatch:             cloudwatch.NewFromConfig(cfg),
		CloudWatchLogs:         cloudwatchlogs.NewFromConfig(cfg),
		ELBv2:                  elasticloadbalancingv2.NewFromConfig(cfg),
		CodeDeploy:             codedeploy.NewFromConfig(cfg),
		ApplicationAutoScaling: applicationautoscaling.NewFromConfig(cfg),
		AutoScaling:            autoscaling.NewFromConfig(cfg),
		STS:                    sts.NewFromConfig(cfg),
		BedrockRuntime:         bedrockruntime.NewFromConfig(cfg),
		Bedrock:                bedrock.NewFromConfig(cfg),
		Region:                 cfg.Region,
	}
}

func NewClient(ctx context.Context, appConfig *Config) (*Client, error) {
	// Synchronous startup path: keep the budget short so the UI appears fast.
	timeoutCtx, cancel := withDefaultTimeout(ctx, 3*time.Second)
	defer cancel()

	if appConfig == nil {
		appConfig = &Config{}
	}

	if cfg, ok := loadCachedAWSConfig(timeoutCtx, currentProfile(), appConfig.Region); ok {
		client := newClientFromConfig(cfg)
		if err := client.loadAccountIdentity(timeoutCtx); err == nil {
			return client, nil
		}
		// Cached creds failed the identity check; fall through to full resolution and resave.
	}

	loadOptions := baseLoadOptions()
	if appConfig.Region != "" {
		loadOptions = append(loadOptions, awsconfig.WithRegion(appConfig.Region))
	}

	cfg, err := awsconfig.LoadDefaultConfig(timeoutCtx, loadOptions...)
	if err != nil {
		return nil, err
	}
	cfg, err = ensureRegion(cfg)
	if err != nil {
		return nil, err
	}
	_ = saveCachedCredentials(timeoutCtx, currentProfile(), cfg)

	client := newClientFromConfig(cfg)

	_ = client.loadAccountIdentity(timeoutCtx)

	return client, nil
}

func NewClientWithProfile(ctx context.Context, profile, region string) (*Client, error) {
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()

	if cfg, ok := loadCachedAWSConfig(timeoutCtx, profile, region); ok {
		client := newClientFromConfig(cfg)
		if err := client.loadAccountIdentity(timeoutCtx); err == nil {
			return client, nil
		}
		// Cached creds failed the identity check; fall through to full resolution and resave.
	}

	loadOptions := append(baseLoadOptions(), awsconfig.WithSharedConfigProfile(profile))
	if region != "" {
		loadOptions = append(loadOptions, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(timeoutCtx, loadOptions...)
	if err != nil {
		return nil, err
	}
	cfg, err = ensureRegion(cfg)
	if err != nil {
		return nil, err
	}
	_ = saveCachedCredentials(timeoutCtx, profile, cfg)

	client := newClientFromConfig(cfg)

	_ = client.loadAccountIdentity(timeoutCtx)

	return client, nil
}

func (c *Client) GetRegion() string {
	return c.Region
}

func (c *Client) GetAccountID() string {
	return c.AccountID
}

// Ready lets panels treat a missing session and an unusable session as the same unavailable state.
func (c *Client) Ready() bool {
	return c != nil && c.STS != nil
}

// AuthError centralizes identity failure so every panel does not repeat the same credentials error.
func (c *Client) AuthError() error {
	if c == nil {
		return errors.New("no AWS session")
	}

	return c.identityErr
}

func (c *Client) RefreshAccountIdentity(ctx context.Context) error {
	return c.loadAccountIdentity(ctx)
}

func (c *Client) loadAccountIdentity(ctx context.Context) error {
	if c.STS == nil {
		return nil
	}

	// Unconditional timeout: Go keeps the earlier of parent/child deadlines, so the validity probe stays bounded even under a larger caller budget.
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.STS.GetCallerIdentity(timeoutCtx, &sts.GetCallerIdentityInput{})
	c.identityErr = err
	if err != nil {
		return err
	}

	if resp.Account != nil {
		c.AccountID = *resp.Account
	}

	return nil
}

func ensureRegion(cfg aws.Config) (aws.Config, error) {
	if cfg.Region == "" {
		return aws.Config{}, fmt.Errorf("no AWS region configured: set region in ~/.aws/config, AWS_REGION, or -region")
	}
	return cfg, nil
}

func withDefaultTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}

func (c *Client) ListECRRepositories(ctx context.Context) ([]string, error) {
	if c.ECR == nil {
		return nil, fmt.Errorf("ECR client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 10*time.Second)
	defer cancel()
	var repos []string
	var nextToken *string
	for {
		out, err := c.ECR.DescribeRepositories(timeoutCtx, &ecr.DescribeRepositoriesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, err
		}
		for _, r := range out.Repositories {
			if r.RepositoryName != nil {
				repos = append(repos, *r.RepositoryName)
			}
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	return repos, nil
}

// sdkLogger routes SDK diagnostics to slog, which the app has already pointed at the debug file or io.Discard.
// The SDK default writes to stderr, and a stray line there scrolls the terminal out from under the TUI mid-frame.
type sdkLogger struct{}

func (sdkLogger) Logf(classification logging.Classification, format string, v ...any) {
	slog.Debug("aws sdk", "classification", string(classification), "message", fmt.Sprintf(format, v...))
}

// baseLoadOptions carries the settings every client shares, whichever profile it resolves.
func baseLoadOptions() []func(*awsconfig.LoadOptions) error {
	return []func(*awsconfig.LoadOptions) error{
		awsconfig.WithLogger(sdkLogger{}),
		awsconfig.WithRetryMode(retryMode),
	}
}

// retryMode adds client-side rate limiting on top of the standard retryer: after a throttle response the adaptive retryer slows the send rate itself instead of retrying into the same limit.
// Refreshing eight panels and an open overview on a timer is many small reads against per-account API quotas, and the standard retryer answers a throttle by retrying, which is what turns one throttled call into a burst.
const retryMode = aws.RetryModeAdaptive
