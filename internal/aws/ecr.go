package aws

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"
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

// chooseBestTag picks the most stable/unique tag for an image digest.
// No semver, no external deps.
func chooseBestTag(tags []string) string {
	if len(tags) == 0 {
		return ""
	}

	// Tags we want to avoid because they point to multiple digests over time
	avoidable := map[string]bool{
		"latest":     true,
		"staging":    true,
		"production": true,
		"prod":       true,
		"dev":        true,
		"test":       true,
		"edge":       true,
		"canary":     true,
	}

	// 1. Pick the first "stable-ish" tag (numbers/dots/hyphens)
	var stable []string
	for _, t := range tags {
		if avoidable[strings.ToLower(t)] {
			continue
		}
		// heuristic: version-ish or release-ish strings
		if strings.ContainsAny(t, "0123456789.") {
			stable = append(stable, t)
		}
	}

	// Prefer stable-looking tag
	if len(stable) > 0 {
		return stable[0]
	}

	// 2. Pick any tag that is not in avoid list
	for _, t := range tags {
		if !avoidable[strings.ToLower(t)] {
			return t
		}
	}

	// 3. Fallback — forced to pick something
	return tags[0]
}

// GetECRImageScan retrieves scan findings for an image digest.
// ECR scans are tied to tags, so we resolve the tag first.
func (c *Client) GetECRImageScan(ctx context.Context, repoName, digest string) (*ECRScanResult, error) {
	if c.ECR == nil {
		return nil, fmt.Errorf("ECR client not initialized")
	}

	// Ensure we have a timeout
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
	}

	// 1. Resolve tags for the digest
	descOut, err := c.ECR.DescribeImages(ctx, &ecr.DescribeImagesInput{
		RepositoryName: aws.String(repoName),
		ImageIds: []ecrTypes.ImageIdentifier{
			{ImageDigest: aws.String(digest)},
		},
	})
	if err != nil || len(descOut.ImageDetails) == 0 {
		return nil, fmt.Errorf("describe image: %w", err)
	}

	details := descOut.ImageDetails[0]
	if len(details.ImageTags) == 0 {
		return nil, fmt.Errorf("image %s has no tag; cannot fetch scan", digest)
	}

	// Choose best tag (maybe a bit overengineered)
	tag := chooseBestTag(details.ImageTags)

	// 2. Retrieve scan findings
	out, err := c.ECR.DescribeImageScanFindings(ctx, &ecr.DescribeImageScanFindingsInput{
		RepositoryName: aws.String(repoName),
		ImageId: &ecrTypes.ImageIdentifier{
			ImageTag: aws.String(tag),
		},
	})
	if err != nil {
		// Provide a friendly explanation for common cause
		if strings.Contains(err.Error(), "ScanNotFoundException") {
			return nil, fmt.Errorf("no scan exists for tag %q; trigger a scan first", tag)
		}
		return nil, err
	}

	// 3. Build result
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

		// Severity map
		maps.Copy(res.SeverityCount, out.ImageScanFindings.FindingSeverityCounts)

		// Limit findings
		findings := out.ImageScanFindings.Findings
		if len(findings) > 10 {
			findings = findings[:10]
		}

		for _, f := range findings {
			fd := ECRScanFinding{
				Name:        getString(f.Name),
				Severity:    string(f.Severity),
				Description: getString(f.Description),
				URI:         getString(f.Uri),
				Attributes:  map[string]string{},
			}
			for _, attr := range f.Attributes {
				if attr.Key != nil && attr.Value != nil {
					fd.Attributes[*attr.Key] = *attr.Value
				}
			}
			res.Findings = append(res.Findings, fd)
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
