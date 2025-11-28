package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smTypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// SecretSummary represents a secret list entry.
type SecretSummary struct {
	Name            string
	Arn             string
	Description     string
	CreatedAt       *time.Time
	LastChanged     *time.Time
	LastAccessed    *time.Time
	RotationEnabled bool
	NextRotation    *time.Time
	PrimaryRegion   string
	Tags            []smTypes.Tag
	HasReplication  bool
	OwningService   string
	KMSKeyID        string
	LastRotated     *time.Time
}

// SecretDetails holds full secret info including value (string only for now).
type SecretDetails struct {
	SecretSummary
	ValueString string
	Versions    []smTypes.SecretVersionsListEntry
	Replication []smTypes.ReplicationStatusType
	Rotation    *smTypes.RotationRulesType
	RotationARN string
	RawJSON     string
}

func (c *Client) ListSecrets(ctx context.Context) ([]SecretSummary, error) {
	if c.Secrets == nil {
		return nil, fmt.Errorf("Secrets Manager client not initialized")
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}

	var items []SecretSummary
	var nextToken *string
	for {
		out, err := c.Secrets.ListSecrets(ctx, &secretsmanager.ListSecretsInput{NextToken: nextToken})
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
				NextRotation:    s.NextRotationDate,
				PrimaryRegion:   getString(s.PrimaryRegion),
				Tags:            s.Tags,
				OwningService:   getString(s.OwningService),
				KMSKeyID:        getString(s.KmsKeyId),
				LastRotated:     s.LastRotatedDate,
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

// GetSecretDetails fetches secret metadata, value, rotation, versions, replication, tags.
func (c *Client) GetSecretDetails(ctx context.Context, name string) (*SecretDetails, error) {
	if c.Secrets == nil {
		return nil, fmt.Errorf("Secrets Manager client not initialized")
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
	}

	desc, err := c.Secrets.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(name)})
	if err != nil {
		return nil, err
	}

	// value
	valOut, err := c.Secrets.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(name)})
	if err != nil {
		return nil, err
	}
	val := ""
	rawJSON := ""
	if valOut.SecretString != nil {
		val = *valOut.SecretString
		// pretty JSON if possible
		var anyJSON interface{}
		if err := json.Unmarshal([]byte(val), &anyJSON); err == nil {
			pretty, _ := json.MarshalIndent(anyJSON, "", "  ")
			rawJSON = string(pretty)
		}
	}

	// versions
	versOut, _ := c.Secrets.ListSecretVersionIds(ctx, &secretsmanager.ListSecretVersionIdsInput{SecretId: aws.String(name), IncludeDeprecated: aws.Bool(true)})
	versions := versOut.Versions
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
			NextRotation:    desc.NextRotationDate,
			PrimaryRegion:   getString(desc.PrimaryRegion),
			Tags:            desc.Tags,
			OwningService:   getString(desc.OwningService),
			KMSKeyID:        getString(desc.KmsKeyId),
			LastRotated:     desc.LastRotatedDate,
		},
		ValueString: val,
		Versions:    versions,
		Replication: desc.ReplicationStatus,
		Rotation:    desc.RotationRules,
		RotationARN: getString(desc.RotationLambdaARN),
		RawJSON:     rawJSON,
	}
	if len(desc.ReplicationStatus) > 0 {
		details.HasReplication = true
	}

	return details, nil
}
