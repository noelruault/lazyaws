package aws

import (
	"context"
	"errors"
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
	Name           string
	Arn            string
	URI            string
	RegistryID     string
	CreatedAt      *time.Time
	ScanOnPush     bool
	TagMutability  string
	EncryptionType string
	KMSKey         string
	// PolicyText and LifecyclePolicy are "" when no policy is attached and "" again when the read failed, which is what the Err fields beside them are for: without checking those first, a renderer states an absence it cannot know.
	// These two are the least reliable fields on the row — the list fetch spends one deadline on the repository pages AND two policy calls per repository, so they are what runs out of budget.
	PolicyText         string
	PolicyErr          error
	LifecyclePolicy    string
	LifecycleEvaluated *time.Time
	LifecyclePolicyErr error
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

			// Optional policy calls must not hide the repository metadata, which is why their failure is carried on the row instead of failing the list: a repository is still worth showing without them.
			pol, polErr := c.ECR.GetRepositoryPolicy(timeoutCtx, &ecr.GetRepositoryPolicyInput{RepositoryName: r.RepositoryName})
			repo.PolicyText, repo.PolicyErr = repositoryPolicyResult(pol, polErr)

			lc, lcErr := c.ECR.GetLifecyclePolicy(timeoutCtx, &ecr.GetLifecyclePolicyInput{RepositoryName: r.RepositoryName})
			repo.LifecyclePolicy, repo.LifecycleEvaluated, repo.LifecyclePolicyErr = lifecyclePolicyResult(lc, lcErr)

			repos = append(repos, repo)
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	return repos, nil
}

// repositoryPolicyResult pairs the policy with the read that produced it, because "none attached" and "could not be read" are both the empty string and only the error tells them apart.
// Returning them together is what stops the error being dropped again: the fetch cannot record one without the other.
// ECR answers an unattached policy WITH an error rather than an empty body, so RepositoryPolicyNotFoundException is the absence and every other error is a read that did not happen.
func repositoryPolicyResult(out *ecr.GetRepositoryPolicyOutput, err error) (string, error) {
	var absent *ecrTypes.RepositoryPolicyNotFoundException
	if errors.As(err, &absent) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if out == nil {
		return "", nil
	}

	return getString(out.PolicyText), nil
}

// lifecyclePolicyResult keeps the last evaluation with the policy it belongs to: a nil stamp means "attached but never evaluated" only when the read succeeded.
// LifecyclePolicyNotFoundException is this call's way of saying no policy is set, so it is the absence and not a failure.
func lifecyclePolicyResult(out *ecr.GetLifecyclePolicyOutput, err error) (string, *time.Time, error) {
	var absent *ecrTypes.LifecyclePolicyNotFoundException
	if errors.As(err, &absent) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	if out == nil {
		return "", nil, nil
	}

	return getString(out.LifecyclePolicyText), out.LastEvaluatedAt, nil
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
