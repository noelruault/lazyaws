package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	secretsmanagertypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

type SecretSummary struct {
	Name            string
	Arn             string
	Description     string
	CreatedAt       *time.Time
	LastChanged     *time.Time
	LastAccessed    *time.Time
	RotationEnabled bool
	// RotationDays is 0 when Secrets Manager reports no day cadence, which is not the same as "does not rotate": a secret scheduled by a cron() or rate() expression only gets AutomaticallyAfterDays filled in after its first successful rotation.
	RotationDays   int64
	NextRotation   *time.Time
	PrimaryRegion  string
	Tags           []secretsmanagertypes.Tag
	HasReplication bool
	OwningService  string
	KMSKeyID       string
	LastRotated    *time.Time
	DeletedDate    *time.Time
}

type SecretDetails struct {
	SecretSummary
	ValueString string
	Versions    []secretsmanagertypes.SecretVersionsListEntry
	Replication []secretsmanagertypes.ReplicationStatusType
	Rotation    *secretsmanagertypes.RotationRulesType
	RotationARN string
	RawJSON     string
	// ResourcePolicy is "" when no policy is attached. A read that failed leaves it empty too, which is what ResourcePolicyErr is for: without checking that first, a renderer states an absence it cannot know.
	ResourcePolicy    string
	ResourcePolicyErr error
}

// rotationDays reads the cadence off a rotation schedule that is absent on every secret that has never had one configured.
func rotationDays(rules *secretsmanagertypes.RotationRulesType) int64 {
	if rules == nil || rules.AutomaticallyAfterDays == nil {
		return 0
	}

	return *rules.AutomaticallyAfterDays
}

func (c *Client) ListSecrets(ctx context.Context, includeDeleted bool) ([]SecretSummary, error) {
	if c.Secrets == nil {
		return nil, fmt.Errorf("Secrets Manager client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()

	var items []SecretSummary
	var nextToken *string
	for {
		out, err := c.Secrets.ListSecrets(timeoutCtx, &secretsmanager.ListSecretsInput{
			NextToken:              nextToken,
			IncludePlannedDeletion: aws.Bool(includeDeleted),
		})
		if err != nil {
			return nil, err
		}
		for _, s := range out.SecretList {
			item := SecretSummary{
				Name:            getString(s.Name),
				Arn:             getString(s.ARN),
				Description:     getString(s.Description),
				CreatedAt:       s.CreatedDate,
				LastChanged:     s.LastChangedDate,
				LastAccessed:    s.LastAccessedDate,
				RotationEnabled: s.RotationEnabled != nil && *s.RotationEnabled,
				RotationDays:    rotationDays(s.RotationRules),
				NextRotation:    s.NextRotationDate,
				PrimaryRegion:   getString(s.PrimaryRegion),
				Tags:            s.Tags,
				OwningService:   getString(s.OwningService),
				KMSKeyID:        getString(s.KmsKeyId),
				LastRotated:     s.LastRotatedDate,
				DeletedDate:     s.DeletedDate,
			}
			items = append(items, item)
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	return items, nil
}

// GetSecretDetails avoids plaintext so metadata views never emit a GetSecretValue CloudTrail data event.
func (c *Client) GetSecretDetails(ctx context.Context, name string) (*SecretDetails, error) {
	if c.Secrets == nil {
		return nil, fmt.Errorf("Secrets Manager client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 20*time.Second)
	defer cancel()

	desc, err := c.Secrets.DescribeSecret(timeoutCtx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(name)})
	if err != nil {
		return nil, err
	}

	// Version listing is best-effort so metadata still renders when it fails.
	var versions []secretsmanagertypes.SecretVersionsListEntry
	versOut, versErr := c.Secrets.ListSecretVersionIds(timeoutCtx, &secretsmanager.ListSecretVersionIdsInput{SecretId: aws.String(name), IncludeDeprecated: aws.Bool(true)})
	if versErr == nil && versOut != nil {
		versions = versOut.Versions
	}
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].CreatedDate == nil {
			return false
		}
		if versions[j].CreatedDate == nil {
			return true
		}
		return versions[i].CreatedDate.After(*versions[j].CreatedDate)
	})

	details := &SecretDetails{
		SecretSummary: SecretSummary{
			Name:            getString(desc.Name),
			Arn:             getString(desc.ARN),
			Description:     getString(desc.Description),
			CreatedAt:       desc.CreatedDate,
			LastChanged:     desc.LastChangedDate,
			LastAccessed:    desc.LastAccessedDate,
			RotationEnabled: desc.RotationEnabled != nil && *desc.RotationEnabled,
			RotationDays:    rotationDays(desc.RotationRules),
			NextRotation:    desc.NextRotationDate,
			PrimaryRegion:   getString(desc.PrimaryRegion),
			Tags:            desc.Tags,
			OwningService:   getString(desc.OwningService),
			KMSKeyID:        getString(desc.KmsKeyId),
			LastRotated:     desc.LastRotatedDate,
			DeletedDate:     desc.DeletedDate,
		},
		Versions:    versions,
		Replication: desc.ReplicationStatus,
		Rotation:    desc.RotationRules,
		RotationARN: getString(desc.RotationLambdaARN),
	}
	if len(desc.ReplicationStatus) > 0 {
		details.HasReplication = true
	}

	// A missing or unreadable resource policy must not fail the metadata view, but the failure is carried rather than dropped:
	// this is the last of three calls sharing timeoutCtx, so it is the one that runs out of budget, and a deadline here is not evidence that no policy is attached.
	policyOut, policyErr := c.Secrets.GetResourcePolicy(timeoutCtx, &secretsmanager.GetResourcePolicyInput{SecretId: aws.String(name)})
	switch {
	case policyErr != nil:
		details.ResourcePolicyErr = policyErr
	case policyOut != nil:
		details.ResourcePolicy = getString(policyOut.ResourcePolicy)
	}

	return details, nil
}

func (c *Client) RotateSecret(ctx context.Context, name string) error {
	if c.Secrets == nil {
		return fmt.Errorf("Secrets Manager client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()

	_, err := c.Secrets.RotateSecret(timeoutCtx, &secretsmanager.RotateSecretInput{
		SecretId:          aws.String(name),
		RotateImmediately: aws.Bool(true),
	})
	return err
}

// DeleteSecret accepts 7-30 recovery days; zero leaves AWS to apply its 30-day default.
func (c *Client) DeleteSecret(ctx context.Context, name string, recoveryWindowDays int64) error {
	if c.Secrets == nil {
		return fmt.Errorf("Secrets Manager client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()

	input := &secretsmanager.DeleteSecretInput{SecretId: aws.String(name)}
	if recoveryWindowDays > 0 {
		input.RecoveryWindowInDays = aws.Int64(recoveryWindowDays)
	}
	_, err := c.Secrets.DeleteSecret(timeoutCtx, input)
	return err
}

func (c *Client) RestoreSecret(ctx context.Context, name string) error {
	if c.Secrets == nil {
		return fmt.Errorf("Secrets Manager client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()

	_, err := c.Secrets.RestoreSecret(timeoutCtx, &secretsmanager.RestoreSecretInput{SecretId: aws.String(name)})
	return err
}

func (c *Client) ReplicateSecretToRegions(ctx context.Context, name string, regions []string) error {
	if c.Secrets == nil {
		return fmt.Errorf("Secrets Manager client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()

	addRegions := make([]secretsmanagertypes.ReplicaRegionType, len(regions))
	for i, r := range regions {
		addRegions[i] = secretsmanagertypes.ReplicaRegionType{Region: aws.String(r)}
	}
	_, err := c.Secrets.ReplicateSecretToRegions(timeoutCtx, &secretsmanager.ReplicateSecretToRegionsInput{
		SecretId:          aws.String(name),
		AddReplicaRegions: addRegions,
	})
	return err
}

func (c *Client) RemoveSecretReplicaRegions(ctx context.Context, name string, regions []string) error {
	if c.Secrets == nil {
		return fmt.Errorf("Secrets Manager client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()

	_, err := c.Secrets.RemoveRegionsFromReplication(timeoutCtx, &secretsmanager.RemoveRegionsFromReplicationInput{
		SecretId:             aws.String(name),
		RemoveReplicaRegions: regions,
	})
	return err
}

// ConfigureSecretRotation updates the schedule without triggering an immediate rotation.
func (c *Client) ConfigureSecretRotation(ctx context.Context, name, lambdaARN string, days int32) error {
	if c.Secrets == nil {
		return fmt.Errorf("Secrets Manager client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()

	_, err := c.Secrets.RotateSecret(timeoutCtx, &secretsmanager.RotateSecretInput{
		SecretId:          aws.String(name),
		RotateImmediately: aws.Bool(false),
		RotationLambdaARN: aws.String(lambdaARN),
		RotationRules:     &secretsmanagertypes.RotationRulesType{AutomaticallyAfterDays: aws.Int64(int64(days))},
	})
	return err
}

// GetSecretValueString returns raw plaintext plus a formatted copy when it is valid JSON.
func (c *Client) GetSecretValueString(ctx context.Context, name string) (value string, prettyJSON string, err error) {
	if c.Secrets == nil {
		return "", "", fmt.Errorf("Secrets Manager client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 20*time.Second)
	defer cancel()

	valOut, err := c.Secrets.GetSecretValue(timeoutCtx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(name)})
	if err != nil {
		return "", "", err
	}
	if valOut.SecretString != nil {
		value = *valOut.SecretString
		prettyJSON = prettySecretJSON(value)
	}
	return value, prettyJSON, nil
}

// prettySecretJSON only re-indents the stored bytes.
// Decoding and re-encoding would rewrite the ampersand and angle brackets as numeric escapes and sort the keys, so a rendered RDS password would no longer be the password.
func prettySecretJSON(value string) string {
	if !json.Valid([]byte(value)) {
		return ""
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(value), "", "  "); err != nil {
		return ""
	}

	return buf.String()
}
