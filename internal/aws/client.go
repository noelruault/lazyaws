package aws

import (
	"context"
	"fmt"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/noelruault/lazyaws/internal/config"
)

// Client wraps AWS service clients
type Client struct {
	EC2            *ec2.Client
	S3             *s3.Client
	EKS            *eks.Client
	ECS            *ecs.Client
	ECR            *ecr.Client
	SSM            *ssm.Client
	CloudWatch     *cloudwatch.Client
	CloudWatchLogs *cloudwatchlogs.Client
	STS            *sts.Client
	Region         string
	AccountID      string
	AccountName    string
}

// NewClient creates a new AWS client with the default configuration
func NewClient(ctx context.Context, appConfig *config.Config) (*Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(appConfig.Region))
	if err != nil {
		return nil, err
	}

	client := &Client{
		EC2:            ec2.NewFromConfig(cfg),
		S3:             s3.NewFromConfig(cfg),
		EKS:            eks.NewFromConfig(cfg),
		ECS:            ecs.NewFromConfig(cfg),
		ECR:            ecr.NewFromConfig(cfg),
		SSM:            ssm.NewFromConfig(cfg),
		CloudWatch:     cloudwatch.NewFromConfig(cfg),
		CloudWatchLogs: cloudwatchlogs.NewFromConfig(cfg),
		STS:            sts.NewFromConfig(cfg),
		Region:         cfg.Region,
	}

	// Try to get account identity
	_ = client.loadAccountIdentity(ctx)

	return client, nil
}

// NewClientWithProfile creates a new AWS client with a specific profile
func NewClientWithProfile(ctx context.Context, profile string) (*Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithSharedConfigProfile(profile),
	)
	if err != nil {
		return nil, err
	}

	client := &Client{
		EC2:            ec2.NewFromConfig(cfg),
		S3:             s3.NewFromConfig(cfg),
		EKS:            eks.NewFromConfig(cfg),
		ECS:            ecs.NewFromConfig(cfg),
		ECR:            ecr.NewFromConfig(cfg),
		SSM:            ssm.NewFromConfig(cfg),
		CloudWatch:     cloudwatch.NewFromConfig(cfg),
		CloudWatchLogs: cloudwatchlogs.NewFromConfig(cfg),
		STS:            sts.NewFromConfig(cfg),
		Region:         cfg.Region,
	}

	// Try to get account identity
	_ = client.loadAccountIdentity(ctx)

	return client, nil
}

// NewClientWithSSOCredentials creates a new AWS client with SSO credentials
func NewClientWithSSOCredentials(ctx context.Context, creds *SSOCredentials, region, accountName string) (*Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			creds.AccessKeyID,
			creds.SecretAccessKey,
			creds.SessionToken,
		)),
	)
	if err != nil {
		return nil, err
	}

	client := &Client{
		EC2:            ec2.NewFromConfig(cfg),
		S3:             s3.NewFromConfig(cfg),
		EKS:            eks.NewFromConfig(cfg),
		ECS:            ecs.NewFromConfig(cfg),
		ECR:            ecr.NewFromConfig(cfg),
		SSM:            ssm.NewFromConfig(cfg),
		CloudWatch:     cloudwatch.NewFromConfig(cfg),
		CloudWatchLogs: cloudwatchlogs.NewFromConfig(cfg),
		STS:            sts.NewFromConfig(cfg),
		Region:         cfg.Region,
		AccountName:    accountName,
	}

	// Get account identity
	_ = client.loadAccountIdentity(ctx)

	return client, nil
}

// GetRegion returns the configured AWS region
func (c *Client) GetRegion() string {
	return c.Region
}

// GetAccountID returns the AWS account ID
func (c *Client) GetAccountID() string {
	return c.AccountID
}

// GetAccountName returns the AWS account name (from SSO)
func (c *Client) GetAccountName() string {
	return c.AccountName
}

// loadAccountIdentity loads the account ID using STS GetCallerIdentity
func (c *Client) loadAccountIdentity(ctx context.Context) error {
	if c.STS == nil {
		return nil
	}

	resp, err := c.STS.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return err
	}

	if resp.Account != nil {
		c.AccountID = *resp.Account
	}

	return nil
}

// ListECRRepositories returns ECR repository names.
func (c *Client) ListECRRepositories(ctx context.Context) ([]string, error) {
	if c.ECR == nil {
		return nil, fmt.Errorf("ECR client not initialized")
	}
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) <= 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	var repos []string
	var nextToken *string
	for {
		out, err := c.ECR.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{
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
