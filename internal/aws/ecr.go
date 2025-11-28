package aws

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrTypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
)

// ECRRepository contains repository metadata and policies.
type ECRRepository struct {
	Name               string
	Arn                string
	URI                string
	RegistryID         string
	CreatedAt          *time.Time
	ScanOnPush         bool
	TagMutability      string
	EncryptionType     string
	KMSKey             string
	PolicyText         string
	LifecyclePolicy    string
	LifecycleEvaluated *time.Time
}

// ECRImage represents a container image in ECR.
type ECRImage struct {
	RepoName     string
	RegistryID   string
	Digest       string
	Tags         []string
	SizeBytes    int64
	PushedAt     *time.Time
	ManifestType string
	ArtifactType string
	Severity     map[string]int32
}

// ECRScanFinding represents a single scan finding.
type ECRScanFinding struct {
	Name        string
	Severity    string
	Description string
	URI         string
	Attributes  map[string]string
}

// ECRScanResult captures scan status and findings.
type ECRScanResult struct {
	Status        string
	Description   string
	CompletedAt   *time.Time
	DBUpdatedAt   *time.Time
	SeverityCount map[string]int32
	Findings      []ECRScanFinding
}

// ListECRRepositoriesDetailed returns repository metadata with optional policy/lifecycle data.
func (c *Client) ListECRRepositoriesDetailed(ctx context.Context) ([]ECRRepository, error) {
	if c.ECR == nil {
		return nil, fmt.Errorf("ECR client not initialized")
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}

	var repos []ECRRepository
	var nextToken *string
	for {
		out, err := c.ECR.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{NextToken: nextToken})
		if err != nil {
			return nil, fmt.Errorf("describe repositories: %w", err)
		}
		for _, r := range out.Repositories {
			repo := ECRRepository{
				Name:          getString(r.RepositoryName),
				Arn:           getString(r.RepositoryArn),
				URI:           getString(r.RepositoryUri),
				RegistryID:    getString(r.RegistryId),
				CreatedAt:     r.CreatedAt,
				TagMutability: string(r.ImageTagMutability),
			}
			if r.ImageScanningConfiguration != nil {
				repo.ScanOnPush = r.ImageScanningConfiguration.ScanOnPush
			}
			if r.EncryptionConfiguration != nil {
				repo.EncryptionType = string(r.EncryptionConfiguration.EncryptionType)
				repo.KMSKey = getString(r.EncryptionConfiguration.KmsKey)
			}

			// Policy (best-effort)
			pol, err := c.ECR.GetRepositoryPolicy(ctx, &ecr.GetRepositoryPolicyInput{RepositoryName: r.RepositoryName})
			if err == nil && pol.PolicyText != nil {
				repo.PolicyText = *pol.PolicyText
			}

			// Lifecycle (best-effort)
			lc, err := c.ECR.GetLifecyclePolicy(ctx, &ecr.GetLifecyclePolicyInput{RepositoryName: r.RepositoryName})
			if err == nil {
				if lc.LifecyclePolicyText != nil {
					repo.LifecyclePolicy = *lc.LifecyclePolicyText
				}
				repo.LifecycleEvaluated = lc.LastEvaluatedAt
			}

			repos = append(repos, repo)
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	return repos, nil
}

// ListECRImages returns images for a repository.
func (c *Client) ListECRImages(ctx context.Context, repoName string) ([]ECRImage, error) {
	if c.ECR == nil {
		return nil, fmt.Errorf("ECR client not initialized")
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}

	var images []ECRImage
	var nextToken *string
	for {
		out, err := c.ECR.DescribeImages(ctx, &ecr.DescribeImagesInput{
			RepositoryName: aws.String(repoName),
			NextToken:      nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("describe images: %w", err)
		}
		for _, img := range out.ImageDetails {
			item := ECRImage{
				RepoName:     repoName,
				RegistryID:   getString(img.RegistryId),
				Digest:       getString(img.ImageDigest),
				Tags:         img.ImageTags,
				SizeBytes:    getInt64Value(img.ImageSizeInBytes),
				PushedAt:     img.ImagePushedAt,
				ManifestType: getString(img.ImageManifestMediaType),
				ArtifactType: getString(img.ArtifactMediaType),
			}
			if img.ImageScanFindingsSummary != nil && img.ImageScanFindingsSummary.FindingSeverityCounts != nil {
				item.Severity = img.ImageScanFindingsSummary.FindingSeverityCounts
			}
			images = append(images, item)
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	// newest first
	sort.Slice(images, func(i, j int) bool {
		if images[i].PushedAt == nil {
			return false
		}
		if images[j].PushedAt == nil {
			return true
		}
		return images[i].PushedAt.After(*images[j].PushedAt)
	})

	return images, nil
}

// GetECRImageScan retrieves scan findings (limited set for UI).
func (c *Client) GetECRImageScan(ctx context.Context, repoName, digest string) (*ECRScanResult, error) {
	if c.ECR == nil {
		return nil, fmt.Errorf("ECR client not initialized")
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
	}

	out, err := c.ECR.DescribeImageScanFindings(ctx, &ecr.DescribeImageScanFindingsInput{
		RepositoryName: aws.String(repoName),
		ImageId: &ecrTypes.ImageIdentifier{
			ImageDigest: aws.String(digest),
		},
	})
	if err != nil {
		return nil, err
	}

	res := &ECRScanResult{
		SeverityCount: map[string]int32{},
	}

	if out.ImageScanStatus != nil {
		res.Status = string(out.ImageScanStatus.Status)
		res.Description = getString(out.ImageScanStatus.Description)
	}

	if out.ImageScanFindings != nil {
		res.CompletedAt = out.ImageScanFindings.ImageScanCompletedAt
		res.DBUpdatedAt = out.ImageScanFindings.VulnerabilitySourceUpdatedAt
		if out.ImageScanFindings.FindingSeverityCounts != nil {
			for k, v := range out.ImageScanFindings.FindingSeverityCounts {
				res.SeverityCount[k] = v
			}
		}
		// limit findings to 10 for UI readability
		limit := len(out.ImageScanFindings.Findings)
		if limit > 10 {
			limit = 10
		}
		for _, f := range out.ImageScanFindings.Findings[:limit] {
			finding := ECRScanFinding{
				Name:        getString(f.Name),
				Severity:    string(f.Severity),
				Description: getString(f.Description),
				URI:         getString(f.Uri),
				Attributes:  map[string]string{},
			}
			for _, attr := range f.Attributes {
				if attr.Key != nil && attr.Value != nil {
					finding.Attributes[*attr.Key] = *attr.Value
				}
			}
			res.Findings = append(res.Findings, finding)
		}
	}

	return res, nil
}

func getInt64Value(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
