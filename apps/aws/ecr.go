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

type ECRScanFinding struct {
	Name        string
	Severity    string
	Description string
	URI         string
	Attributes  map[string]string
}

type ECRScanResult struct {
	Status           string
	Description      string
	CompletedAt      *time.Time
	DBUpdatedAt      *time.Time
	SeverityCount    map[string]int32
	Findings         []ECRScanFinding
	EnhancedFindings []ECREnhancedFinding
}

type ECREnhancedFinding struct {
	Title              string
	Severity           string
	CVSSScore          float64
	FixAvailable       string // YES, NO, PARTIAL, or "" (unknown)
	VulnerablePackages []string
}

type ECRLifecyclePolicyPreviewImage struct {
	Tags   []string
	Digest string
	Action string
}

type ECRLifecyclePolicyPreview struct {
	Status             string
	ExpiringImageCount int32
	ExpiringImages     []ECRLifecyclePolicyPreviewImage
}

func (c *Client) ListECRRepositoriesDetailed(ctx context.Context) ([]ECRRepository, error) {
	if c.ECR == nil {
		return nil, fmt.Errorf("ECR client not initialized")
	}

	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()

	var repos []ECRRepository
	var nextToken *string
	for {
		out, err := c.ECR.DescribeRepositories(timeoutCtx, &ecr.DescribeRepositoriesInput{NextToken: nextToken})
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

			// Optional policy calls must not hide the repository metadata.
			pol, err := c.ECR.GetRepositoryPolicy(timeoutCtx, &ecr.GetRepositoryPolicyInput{RepositoryName: r.RepositoryName})
			if err == nil && pol.PolicyText != nil {
				repo.PolicyText = *pol.PolicyText
			}

			lc, err := c.ECR.GetLifecyclePolicy(timeoutCtx, &ecr.GetLifecyclePolicyInput{RepositoryName: r.RepositoryName})
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

func (c *Client) ListECRImages(ctx context.Context, repoName string) ([]ECRImage, error) {
	if c.ECR == nil {
		return nil, fmt.Errorf("ECR client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()

	var images []ECRImage
	var nextToken *string
	for {
		out, err := c.ECR.DescribeImages(timeoutCtx, &ecr.DescribeImagesInput{
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

func chooseBestTag(tags []string) string {
	if len(tags) == 0 {
		return ""
	}

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

	var stable []string
	for _, t := range tags {
		if avoidable[strings.ToLower(t)] {
			continue
		}
		if strings.ContainsAny(t, "0123456789.") {
			stable = append(stable, t)
		}
	}

	if len(stable) > 0 {
		return stable[0]
	}

	for _, t := range tags {
		if !avoidable[strings.ToLower(t)] {
			return t
		}
	}

	return tags[0]
}

// GetECRImageScan resolves a tag first because ECR scans are tag-bound.
func (c *Client) GetECRImageScan(ctx context.Context, repoName, digest string) (*ECRScanResult, error) {
	if c.ECR == nil {
		return nil, fmt.Errorf("ECR client not initialized")
	}

	timeoutCtx, cancel := withDefaultTimeout(ctx, 20*time.Second)
	defer cancel()

	descOut, err := c.ECR.DescribeImages(timeoutCtx, &ecr.DescribeImagesInput{
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

	tag := chooseBestTag(details.ImageTags)

	out, err := c.ECR.DescribeImageScanFindings(timeoutCtx, &ecr.DescribeImageScanFindingsInput{
		RepositoryName: aws.String(repoName),
		ImageId: &ecrTypes.ImageIdentifier{
			ImageTag: aws.String(tag),
		},
	})
	if err != nil {
		if strings.Contains(err.Error(), "ScanNotFoundException") {
			return nil, fmt.Errorf("no scan exists for tag %q; trigger a scan first", tag)
		}
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

		maps.Copy(res.SeverityCount, out.ImageScanFindings.FindingSeverityCounts)

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

		enhanced := out.ImageScanFindings.EnhancedFindings
		if len(enhanced) > 10 {
			enhanced = enhanced[:10]
		}
		for _, ef := range enhanced {
			finding := ECREnhancedFinding{
				Title:        getString(ef.Title),
				Severity:     getString(ef.Severity),
				FixAvailable: getString(ef.FixAvailable),
				CVSSScore:    ef.Score,
			}
			if ef.PackageVulnerabilityDetails != nil {
				if len(ef.PackageVulnerabilityDetails.Cvss) > 0 {
					finding.CVSSScore = ef.PackageVulnerabilityDetails.Cvss[0].BaseScore
				}
				for _, pkg := range ef.PackageVulnerabilityDetails.VulnerablePackages {
					name := getString(pkg.Name)
					if path := getString(pkg.FilePath); path != "" {
						name = fmt.Sprintf("%s (%s)", name, path)
					}
					finding.VulnerablePackages = append(finding.VulnerablePackages, name)
				}
			}
			res.EnhancedFindings = append(res.EnhancedFindings, finding)
		}
	}

	return res, nil
}

// PreviewLifecyclePolicy runs ECR's async dry-run lifecycle policy check (start the preview job, poll until it leaves IN_PROGRESS); mutates nothing.
// Passing an empty policyText previews the repository's currently saved policy.
func (c *Client) PreviewLifecyclePolicy(ctx context.Context, repoName, policyText string) (*ECRLifecyclePolicyPreview, error) {
	if c.ECR == nil {
		return nil, fmt.Errorf("ECR client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 30*time.Second)
	defer cancel()

	startInput := &ecr.StartLifecyclePolicyPreviewInput{RepositoryName: aws.String(repoName)}
	if policyText != "" {
		startInput.LifecyclePolicyText = aws.String(policyText)
	}
	if _, err := c.ECR.StartLifecyclePolicyPreview(timeoutCtx, startInput); err != nil {
		return nil, fmt.Errorf("start lifecycle policy preview: %w", err)
	}

	var out *ecr.GetLifecyclePolicyPreviewOutput
	for {
		var err error
		out, err = c.ECR.GetLifecyclePolicyPreview(timeoutCtx, &ecr.GetLifecyclePolicyPreviewInput{RepositoryName: aws.String(repoName)})
		if err != nil {
			return nil, fmt.Errorf("get lifecycle policy preview: %w", err)
		}
		if out.Status != ecrTypes.LifecyclePolicyPreviewStatusInProgress {
			break
		}
		select {
		case <-timeoutCtx.Done():
			return nil, timeoutCtx.Err()
		case <-time.After(time.Second):
		}
	}

	result := &ECRLifecyclePolicyPreview{Status: string(out.Status)}
	if out.Summary != nil && out.Summary.ExpiringImageTotalCount != nil {
		result.ExpiringImageCount = *out.Summary.ExpiringImageTotalCount
	}
	for _, r := range out.PreviewResults {
		if r.Action == nil || r.Action.Type != ecrTypes.ImageActionTypeExpire {
			continue
		}
		result.ExpiringImages = append(result.ExpiringImages, ECRLifecyclePolicyPreviewImage{
			Tags:   r.ImageTags,
			Digest: getString(r.ImageDigest),
			Action: string(r.Action.Type),
		})
	}
	return result, nil
}

func (c *Client) PutECRLifecyclePolicy(ctx context.Context, repoName, policyText string) error {
	if c.ECR == nil {
		return fmt.Errorf("ECR client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()
	_, err := c.ECR.PutLifecyclePolicy(timeoutCtx, &ecr.PutLifecyclePolicyInput{
		RepositoryName:      aws.String(repoName),
		LifecyclePolicyText: aws.String(policyText),
	})
	if err != nil {
		return fmt.Errorf("put lifecycle policy: %w", err)
	}
	return nil
}

// DeleteECRRepository requires force=true when the repository still holds images (AWS refuses non-empty deletes otherwise).
func (c *Client) DeleteECRRepository(ctx context.Context, repoName string, force bool) error {
	if c.ECR == nil {
		return fmt.Errorf("ECR client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()
	_, err := c.ECR.DeleteRepository(timeoutCtx, &ecr.DeleteRepositoryInput{
		RepositoryName: aws.String(repoName),
		Force:          force,
	})
	if err != nil {
		return fmt.Errorf("delete repository: %w", err)
	}
	return nil
}

func (c *Client) DeleteECRImage(ctx context.Context, repoName, digest string) error {
	if c.ECR == nil {
		return fmt.Errorf("ECR client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := c.ECR.BatchDeleteImage(timeoutCtx, &ecr.BatchDeleteImageInput{
		RepositoryName: aws.String(repoName),
		ImageIds:       []ecrTypes.ImageIdentifier{{ImageDigest: aws.String(digest)}},
	})
	if err != nil {
		return fmt.Errorf("delete image: %w", err)
	}
	if len(out.Failures) > 0 {
		return fmt.Errorf("delete image failed: %s", getString(out.Failures[0].FailureReason))
	}
	return nil
}

func (c *Client) SetImageTagMutability(ctx context.Context, repoName string, immutable bool) error {
	if c.ECR == nil {
		return fmt.Errorf("ECR client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()
	tagMutability := ecrTypes.ImageTagMutabilityMutable
	if immutable {
		tagMutability = ecrTypes.ImageTagMutabilityImmutable
	}
	_, err := c.ECR.PutImageTagMutability(timeoutCtx, &ecr.PutImageTagMutabilityInput{
		RepositoryName:     aws.String(repoName),
		ImageTagMutability: tagMutability,
	})
	if err != nil {
		return fmt.Errorf("set image tag mutability: %w", err)
	}
	return nil
}

func (c *Client) SetScanOnPush(ctx context.Context, repoName string, enable bool) error {
	if c.ECR == nil {
		return fmt.Errorf("ECR client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()
	_, err := c.ECR.PutImageScanningConfiguration(timeoutCtx, &ecr.PutImageScanningConfigurationInput{
		RepositoryName: aws.String(repoName),
		ImageScanningConfiguration: &ecrTypes.ImageScanningConfiguration{
			ScanOnPush: enable,
		},
	})
	if err != nil {
		return fmt.Errorf("set scan on push: %w", err)
	}
	return nil
}

func getInt64Value(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
