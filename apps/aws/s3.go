package aws

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// isAPIErrorCode handles missing S3 configuration reported only through generic Smithy errors.
func isAPIErrorCode(err error, code string) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == code
}

type Bucket struct {
	Name         string
	CreationDate string
	Region       string
	Size         int64
	ObjectCount  int64
}

type S3Object struct {
	Key          string
	Size         int64
	LastModified string
	StorageClass string
	IsFolder     bool
}

type S3ListResult struct {
	Objects               []S3Object
	NextContinuationToken *string
	IsTruncated           bool
}

type S3ObjectDetails struct {
	Key                  string
	Size                 int64
	LastModified         string
	StorageClass         string
	ContentType          string
	ETag                 string
	Metadata             map[string]string
	Tags                 map[string]string
	ServerSideEncryption string // e.g. "AES256" or "aws:kms"
	LegalHoldStatus      string // "ON" or "OFF" or ""
	RetentionMode        string // "GOVERNANCE" or "COMPLIANCE" or ""
	RetentionUntilDate   string // ISO 8601 date or ""
}

type BucketEncryption struct {
	Algorithm string // "AES256" or "aws:kms"
	KMSKeyID  string // KMS key ID (empty if using AES256)
}

type ObjectLockConfiguration struct {
	Enabled              bool
	DefaultRetentionMode string // "GOVERNANCE", "COMPLIANCE", or ""
	DefaultRetentionDays int32  // days until expiration
}

type BucketLogging struct {
	TargetBucket string
	TargetPrefix string // Prefix for log objects (optional)
}

type ProgressCallback func(bytesTransferred int64, totalBytes int64)

type transferProgressListener struct {
	callback ProgressCallback
}

func (listener transferProgressListener) OnObjectBytesTransferred(_ context.Context, event *transfermanager.ObjectBytesTransferredEvent) {
	listener.callback(event.BytesTransferred, event.TotalBytes)
}

func newTransferManager(client *s3.Client, progressCallback ProgressCallback) *transfermanager.Client {
	return transfermanager.New(client, func(options *transfermanager.Options) {
		options.PartSizeBytes = 10 * 1024 * 1024
		if progressCallback != nil {
			options.ObjectProgressListeners.Register(transferProgressListener{callback: progressCallback})
		}
	})
}

func (c *Client) ListBuckets(ctx context.Context) ([]Bucket, error) {
	input := &s3.ListBucketsInput{}
	result, err := c.S3.ListBuckets(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}

	var buckets []Bucket
	for _, bucket := range result.Buckets {
		b := Bucket{
			Name: getString(bucket.Name),
		}

		if bucket.CreationDate != nil {
			b.CreationDate = bucket.CreationDate.Format("2006-01-02 15:04:05")
		}

		b.Region = "-"

		buckets = append(buckets, b)
	}

	return buckets, nil
}

func (c *Client) GetBucketRegion(ctx context.Context, bucketName string) (string, error) {
	input := &s3.GetBucketLocationInput{
		Bucket: &bucketName,
	}

	result, err := c.S3.GetBucketLocation(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to get bucket location: %w", err)
	}

	return bucketRegionFromLocationConstraint(string(result.LocationConstraint)), nil
}

// bucketRegionFromLocationConstraint maps S3's empty constraint to us-east-1.
func bucketRegionFromLocationConstraint(constraint string) string {
	if constraint == "" {
		return "us-east-1"
	}
	return constraint
}

func (c *Client) ListObjects(ctx context.Context, bucketName, prefix string, continuationToken *string) (*S3ListResult, error) {
	delimiter := "/"
	input := &s3.ListObjectsV2Input{
		Bucket:    &bucketName,
		Prefix:    &prefix,
		Delimiter: &delimiter, // Required for S3 to return CommonPrefixes.
		MaxKeys:   getInt32(1000),
	}

	if continuationToken != nil {
		input.ContinuationToken = continuationToken
	}

	result, err := c.S3.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	var objects []S3Object

	for _, commonPrefix := range result.CommonPrefixes {
		if commonPrefix.Prefix != nil {
			folderKey := getString(commonPrefix.Prefix)
			objects = append(objects, S3Object{
				Key:      folderKey,
				IsFolder: true,
			})
		}
	}

	for _, obj := range result.Contents {
		key := getString(obj.Key)

		if key == prefix {
			continue
		}

		storageClass := ""
		if obj.StorageClass != "" {
			storageClass = string(obj.StorageClass)
		} else {
			storageClass = string(types.ObjectStorageClassStandard)
		}

		lastModified := ""
		if obj.LastModified != nil {
			lastModified = obj.LastModified.Format("2006-01-02 15:04:05")
		}

		objects = append(objects, S3Object{
			Key:          key,
			Size:         getInt64(obj.Size),
			LastModified: lastModified,
			StorageClass: storageClass,
			IsFolder:     false,
		})
	}

	return &S3ListResult{
		Objects:               objects,
		NextContinuationToken: result.NextContinuationToken,
		IsTruncated:           getBool(result.IsTruncated),
	}, nil
}

func getBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func getInt64(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
}

func getInt32(i int32) *int32 {
	return &i
}

func (c *Client) DownloadObject(ctx context.Context, bucketName, key, localPath string) error {
	return c.DownloadObjectWithProgress(ctx, bucketName, key, localPath, nil)
}

func (c *Client) DownloadObjectWithProgress(ctx context.Context, bucketName, key, localPath string, progressCallback ProgressCallback) error {
	transferCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		transferCtx, cancel = context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
	}

	destDir := filepath.Dir(localPath)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create download directory %s: %w", destDir, err)
	}

	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer file.Close()

	_, err = newTransferManager(c.S3, progressCallback).DownloadObject(transferCtx, &transfermanager.DownloadObjectInput{
		Bucket:   &bucketName,
		Key:      &key,
		WriterAt: file,
	})
	if err != nil {
		return fmt.Errorf("failed to download object: %w", err)
	}

	return nil
}

func (c *Client) UploadObject(ctx context.Context, bucketName, key, localPath string) error {
	return c.UploadObjectWithProgress(ctx, bucketName, key, localPath, nil)
}

func (c *Client) UploadObjectWithProgress(ctx context.Context, bucketName, key, localPath string, progressCallback ProgressCallback) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer file.Close()

	_, err = newTransferManager(c.S3, progressCallback).UploadObject(ctx, &transfermanager.UploadObjectInput{
		Bucket: &bucketName,
		Key:    &key,
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("failed to upload object: %w", err)
	}

	return nil
}

func (c *Client) GetObjectDetails(ctx context.Context, bucketName, key string) (*S3ObjectDetails, error) {
	headInput := &s3.HeadObjectInput{
		Bucket: &bucketName,
		Key:    &key,
	}

	headResult, err := c.S3.HeadObject(ctx, headInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get object metadata: %w", err)
	}

	details := &S3ObjectDetails{
		Key:      key,
		Size:     getInt64(headResult.ContentLength),
		Metadata: headResult.Metadata,
	}

	if headResult.LastModified != nil {
		details.LastModified = headResult.LastModified.Format("2006-01-02 15:04:05")
	}

	if headResult.StorageClass != "" {
		details.StorageClass = string(headResult.StorageClass)
	} else {
		details.StorageClass = string(types.ObjectStorageClassStandard)
	}

	if headResult.ContentType != nil {
		details.ContentType = *headResult.ContentType
	}

	if headResult.ETag != nil {
		details.ETag = *headResult.ETag
	}

	if headResult.ServerSideEncryption != "" {
		details.ServerSideEncryption = string(headResult.ServerSideEncryption)
	}

	if headResult.ObjectLockLegalHoldStatus != "" {
		details.LegalHoldStatus = string(headResult.ObjectLockLegalHoldStatus)
	}
	if headResult.ObjectLockMode != "" {
		details.RetentionMode = string(headResult.ObjectLockMode)
	}
	if headResult.ObjectLockRetainUntilDate != nil {
		details.RetentionUntilDate = headResult.ObjectLockRetainUntilDate.Format("2006-01-02")
	}

	tagInput := &s3.GetObjectTaggingInput{
		Bucket: &bucketName,
		Key:    &key,
	}

	tagResult, err := c.S3.GetObjectTagging(ctx, tagInput)
	if err == nil && len(tagResult.TagSet) > 0 {
		details.Tags = make(map[string]string)
		for _, tag := range tagResult.TagSet {
			if tag.Key != nil && tag.Value != nil {
				details.Tags[*tag.Key] = *tag.Value
			}
		}
	}

	return details, nil
}

func (c *Client) DeleteObject(ctx context.Context, bucketName, key string) error {
	input := &s3.DeleteObjectInput{
		Bucket: &bucketName,
		Key:    &key,
	}

	_, err := c.S3.DeleteObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	return nil
}

func (c *Client) CopyObject(ctx context.Context, sourceBucket, sourceKey, destBucket, destKey string) error {
	copySource := fmt.Sprintf("%s/%s", sourceBucket, sourceKey)
	input := &s3.CopyObjectInput{
		Bucket:     &destBucket,
		CopySource: &copySource,
		Key:        &destKey,
	}

	_, err := c.S3.CopyObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to copy object: %w", err)
	}

	return nil
}

func s3VersionedCopySource(bucket, key, versionID string) string {
	return fmt.Sprintf("%s/%s?versionId=%s", bucket, key, versionID)
}

// CopyObjectVersion restores by copying because S3 has no native version-restore operation.
func (c *Client) CopyObjectVersion(ctx context.Context, bucketName, key, versionID string) error {
	copySource := s3VersionedCopySource(bucketName, key, versionID)
	input := &s3.CopyObjectInput{
		Bucket:     &bucketName,
		CopySource: &copySource,
		Key:        &key,
	}

	_, err := c.S3.CopyObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to restore object version: %w", err)
	}

	return nil
}

func (c *Client) CreateBucket(ctx context.Context, bucketName, region string) error {
	input := &s3.CreateBucketInput{
		Bucket: &bucketName,
	}

	// us-east-1 rejects an explicit location constraint
	if region != "" && region != "us-east-1" {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(region),
		}
	}

	_, err := c.S3.CreateBucket(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	return nil
}

func (c *Client) DeleteBucket(ctx context.Context, bucketName string) error {
	input := &s3.DeleteBucketInput{
		Bucket: &bucketName,
	}

	_, err := c.S3.DeleteBucket(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete bucket: %w", err)
	}

	return nil
}

// GetBucketPolicy returns "", nil when S3 reports that no policy is attached.
func (c *Client) GetBucketPolicy(ctx context.Context, bucketName string) (string, error) {
	input := &s3.GetBucketPolicyInput{
		Bucket: &bucketName,
	}

	result, err := c.S3.GetBucketPolicy(ctx, input)
	if err != nil {
		if isAPIErrorCode(err, "NoSuchBucketPolicy") {
			return "", nil
		}
		return "", fmt.Errorf("failed to get bucket policy: %w", err)
	}

	if result.Policy == nil {
		return "", nil
	}

	return *result.Policy, nil
}

type PublicAccessBlock struct {
	BlockPublicAcls       bool
	IgnorePublicAcls      bool
	BlockPublicPolicy     bool
	RestrictPublicBuckets bool
}

// GetBucketPublicAccessBlock returns nil, nil when S3 reports that no block configuration is set.
func (c *Client) GetBucketPublicAccessBlock(ctx context.Context, bucketName string) (*PublicAccessBlock, error) {
	input := &s3.GetPublicAccessBlockInput{
		Bucket: &bucketName,
	}

	result, err := c.S3.GetPublicAccessBlock(ctx, input)
	if err != nil {
		if isAPIErrorCode(err, "NoSuchPublicAccessBlockConfiguration") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get bucket public access block: %w", err)
	}
	if result.PublicAccessBlockConfiguration == nil {
		return nil, nil
	}

	cfg := result.PublicAccessBlockConfiguration
	return &PublicAccessBlock{
		BlockPublicAcls:       getBool(cfg.BlockPublicAcls),
		IgnorePublicAcls:      getBool(cfg.IgnorePublicAcls),
		BlockPublicPolicy:     getBool(cfg.BlockPublicPolicy),
		RestrictPublicBuckets: getBool(cfg.RestrictPublicBuckets),
	}, nil
}

func (c *Client) GetBucketVersioning(ctx context.Context, bucketName string) (string, error) {
	input := &s3.GetBucketVersioningInput{
		Bucket: &bucketName,
	}

	result, err := c.S3.GetBucketVersioning(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to get bucket versioning: %w", err)
	}

	if result.Status == "" {
		return "Disabled", nil
	}

	return string(result.Status), nil
}

func (c *Client) GetBucketEncryption(ctx context.Context, bucketName string) (*BucketEncryption, error) {
	input := &s3.GetBucketEncryptionInput{
		Bucket: &bucketName,
	}

	result, err := c.S3.GetBucketEncryption(ctx, input)
	if err != nil {
		if isAPIErrorCode(err, "ServerSideEncryptionConfigurationNotFoundError") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get bucket encryption: %w", err)
	}

	if result.ServerSideEncryptionConfiguration == nil || len(result.ServerSideEncryptionConfiguration.Rules) == 0 {
		return nil, nil
	}

	rule := result.ServerSideEncryptionConfiguration.Rules[0]
	if rule.ApplyServerSideEncryptionByDefault == nil {
		return nil, nil
	}

	enc := rule.ApplyServerSideEncryptionByDefault
	return &BucketEncryption{
		Algorithm: string(enc.SSEAlgorithm),
		KMSKeyID:  getString(enc.KMSMasterKeyID),
	}, nil
}

func (c *Client) GetBucketObjectLockConfiguration(ctx context.Context, bucketName string) (*ObjectLockConfiguration, error) {
	input := &s3.GetObjectLockConfigurationInput{
		Bucket: &bucketName,
	}

	result, err := c.S3.GetObjectLockConfiguration(ctx, input)
	if err != nil {
		if isAPIErrorCode(err, "ObjectLockConfigurationNotFoundError") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get object lock configuration: %w", err)
	}

	if result.ObjectLockConfiguration == nil {
		return nil, nil
	}

	cfg := result.ObjectLockConfiguration
	config := &ObjectLockConfiguration{
		Enabled: cfg.ObjectLockEnabled == types.ObjectLockEnabledEnabled,
	}

	if cfg.Rule != nil && cfg.Rule.DefaultRetention != nil {
		dr := cfg.Rule.DefaultRetention
		config.DefaultRetentionMode = string(dr.Mode)
		if dr.Days != nil {
			config.DefaultRetentionDays = *dr.Days
		}
	}

	return config, nil
}

// GetBucketLogging returns nil, nil when logging is not configured.
func (c *Client) GetBucketLogging(ctx context.Context, bucketName string) (*BucketLogging, error) {
	input := &s3.GetBucketLoggingInput{
		Bucket: &bucketName,
	}

	result, err := c.S3.GetBucketLogging(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket logging: %w", err)
	}

	if result.LoggingEnabled == nil {
		return nil, nil
	}

	return &BucketLogging{
		TargetBucket: getString(result.LoggingEnabled.TargetBucket),
		TargetPrefix: getString(result.LoggingEnabled.TargetPrefix),
	}, nil
}

// GetBucketTagging returns nil, nil when S3 reports that the bucket has no tags.
func (c *Client) GetBucketTagging(ctx context.Context, bucketName string) (map[string]string, error) {
	input := &s3.GetBucketTaggingInput{
		Bucket: &bucketName,
	}

	result, err := c.S3.GetBucketTagging(ctx, input)
	if isAPIErrorCode(err, "NoSuchTagSet") {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket tagging: %w", err)
	}

	if len(result.TagSet) == 0 {
		return nil, nil
	}

	tags := make(map[string]string, len(result.TagSet))
	for _, tag := range result.TagSet {
		if tag.Key != nil && tag.Value != nil {
			tags[*tag.Key] = *tag.Value
		}
	}
	return tags, nil
}

func (c *Client) GeneratePresignedURL(ctx context.Context, bucketName, key string, expirationSeconds int) (string, error) {
	presignClient := s3.NewPresignClient(c.S3)

	input := &s3.GetObjectInput{
		Bucket: &bucketName,
		Key:    &key,
	}

	presignResult, err := presignClient.PresignGetObject(ctx, input, func(opts *s3.PresignOptions) {
		opts.Expires = time.Duration(expirationSeconds) * time.Second
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignResult.URL, nil
}

// GetBucketSize scans every object, so it can be slow for large buckets.
func (c *Client) GetBucketSize(ctx context.Context, bucketName string) (int64, int64, error) {
	var totalSize int64
	var objectCount int64
	var continuationToken *string

	for {
		input := &s3.ListObjectsV2Input{
			Bucket: &bucketName,
		}

		if continuationToken != nil {
			input.ContinuationToken = continuationToken
		}

		result, err := c.S3.ListObjectsV2(ctx, input)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to list objects for size calculation: %w", err)
		}

		for _, obj := range result.Contents {
			totalSize += getInt64(obj.Size)
			objectCount++
		}

		if !getBool(result.IsTruncated) {
			break
		}

		continuationToken = result.NextContinuationToken
	}

	return totalSize, objectCount, nil
}

func (c *Client) ListObjectsWithFilter(ctx context.Context, bucketName, prefix, pattern string, continuationToken *string) (*S3ListResult, error) {
	result, err := c.ListObjects(ctx, bucketName, prefix, continuationToken)
	if err != nil {
		return nil, err
	}

	if pattern == "" {
		return result, nil
	}

	var filteredObjects []S3Object
	for _, obj := range result.Objects {
		if containsIgnoreCase(obj.Key, pattern) {
			filteredObjects = append(filteredObjects, obj)
		}
	}

	result.Objects = filteredObjects
	return result, nil
}

func containsIgnoreCase(s, substr string) bool {
	s = toLower(s)
	substr = toLower(substr)
	return contains(s, substr)
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c + ('a' - 'A')
		}
		result[i] = c
	}
	return string(result)
}

type BucketReplication struct {
	Rules []ReplicationRule
}

type ReplicationRule struct {
	ID                string
	Status            string // Enabled or Disabled
	DestinationBucket string // Destination bucket name (ARN)
	DestinationRegion string // Destination bucket region (extracted from ARN or config)
	ReplicationType   string // ALL or filter-based
}

type LifecycleConfiguration struct {
	Rules []LifecycleRule
}

type LifecycleRule struct {
	ID          string
	Status      string // Enabled or Disabled
	Prefix      string // Prefix filter (if any)
	Filter      string // Filter description (if complex filter used)
	Transitions []Transition
	Expiration  ExpirationAge // Expiration days or date
}

type Transition struct {
	StorageClass string // Storage class name (GLACIER, DEEP_ARCHIVE, etc)
	Days         int    // Days until transition (0 if uses date)
	Date         string // Effective date of transition (if date-based)
}

type ExpirationAge struct {
	Days int    // Days until expiration (0 if uses date)
	Date string // Expiration date (if date-based)
}

type NotificationConfig struct {
	LambdaFunctions []LambdaNotification
	Topics          []SNSNotification
	Queues          []SQSNotification
}

type LambdaNotification struct {
	ID       string
	Function string
	Events   []string
	Filter   string // filter key prefix and suffix, if any
}

type SNSNotification struct {
	ID     string
	Topic  string
	Events []string
	Filter string
}

type SQSNotification struct {
	ID     string
	Queue  string
	Events []string
	Filter string
}

// GetBucketNotificationConfiguration returns nil, nil when no notifications are configured.
func (c *Client) GetBucketNotificationConfiguration(ctx context.Context, bucketName string) (*NotificationConfig, error) {
	input := &s3.GetBucketNotificationConfigurationInput{
		Bucket: &bucketName,
	}

	result, err := c.S3.GetBucketNotificationConfiguration(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket notification configuration: %w", err)
	}

	cfg := &NotificationConfig{}

	for _, lambda := range result.LambdaFunctionConfigurations {
		events := make([]string, len(lambda.Events))
		for i, e := range lambda.Events {
			events[i] = string(e)
		}
		filter := ""
		if lambda.Filter != nil && lambda.Filter.Key != nil {
			if lambda.Filter.Key.FilterRules != nil && len(lambda.Filter.Key.FilterRules) > 0 {
				for _, rule := range lambda.Filter.Key.FilterRules {
					if rule.Name != "" && rule.Value != nil && *rule.Value != "" {
						filter = fmt.Sprintf("%s=%s", rule.Name, *rule.Value)
					}
				}
			}
		}
		cfg.LambdaFunctions = append(cfg.LambdaFunctions, LambdaNotification{
			ID:       getString(lambda.Id),
			Function: getString(lambda.LambdaFunctionArn),
			Events:   events,
			Filter:   filter,
		})
	}

	for _, topic := range result.TopicConfigurations {
		events := make([]string, len(topic.Events))
		for i, e := range topic.Events {
			events[i] = string(e)
		}
		filter := ""
		if topic.Filter != nil && topic.Filter.Key != nil {
			if topic.Filter.Key.FilterRules != nil && len(topic.Filter.Key.FilterRules) > 0 {
				for _, rule := range topic.Filter.Key.FilterRules {
					if rule.Name != "" && rule.Value != nil && *rule.Value != "" {
						filter = fmt.Sprintf("%s=%s", rule.Name, *rule.Value)
					}
				}
			}
		}
		cfg.Topics = append(cfg.Topics, SNSNotification{
			ID:     getString(topic.Id),
			Topic:  getString(topic.TopicArn),
			Events: events,
			Filter: filter,
		})
	}

	for _, queue := range result.QueueConfigurations {
		events := make([]string, len(queue.Events))
		for i, e := range queue.Events {
			events[i] = string(e)
		}
		filter := ""
		if queue.Filter != nil && queue.Filter.Key != nil {
			if queue.Filter.Key.FilterRules != nil && len(queue.Filter.Key.FilterRules) > 0 {
				for _, rule := range queue.Filter.Key.FilterRules {
					if rule.Name != "" && rule.Value != nil && *rule.Value != "" {
						filter = fmt.Sprintf("%s=%s", rule.Name, *rule.Value)
					}
				}
			}
		}
		cfg.Queues = append(cfg.Queues, SQSNotification{
			ID:     getString(queue.Id),
			Queue:  getString(queue.QueueArn),
			Events: events,
			Filter: filter,
		})
	}

	if len(cfg.LambdaFunctions) == 0 && len(cfg.Topics) == 0 && len(cfg.Queues) == 0 {
		return nil, nil
	}

	return cfg, nil
}

// GetBucketReplication returns nil, nil when no replication is configured.
func (c *Client) GetBucketReplication(ctx context.Context, bucketName string) (*BucketReplication, error) {
	input := &s3.GetBucketReplicationInput{
		Bucket: &bucketName,
	}

	result, err := c.S3.GetBucketReplication(ctx, input)
	if isAPIErrorCode(err, "ReplicationConfigurationNotFoundError") {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket replication configuration: %w", err)
	}

	rep := &BucketReplication{
		Rules: make([]ReplicationRule, len(result.ReplicationConfiguration.Rules)),
	}

	for i, rule := range result.ReplicationConfiguration.Rules {
		destBucketARN := getString(rule.Destination.Bucket)
		// S3 destination ARNs omit the region, so v1 avoids a per-rule lookup and reports "remote".
		destRegion := "remote"
		if destBucketARN == "" {
			destRegion = "unknown"
		}

		status := "Unknown"
		if rule.Status != "" {
			status = string(rule.Status)
		}

		rep.Rules[i] = ReplicationRule{
			ID:                getString(rule.ID),
			Status:            status,
			DestinationBucket: destBucketARN,
			DestinationRegion: destRegion,
			ReplicationType:   "All", // v1 does not inspect replication filters.
		}
	}

	if len(rep.Rules) == 0 {
		return nil, nil
	}

	return rep, nil
}

// GetBucketLifecycleConfiguration returns nil, nil when no lifecycle configuration is set.
func (c *Client) GetBucketLifecycleConfiguration(ctx context.Context, bucketName string) (*LifecycleConfiguration, error) {
	input := &s3.GetBucketLifecycleConfigurationInput{
		Bucket: &bucketName,
	}

	result, err := c.S3.GetBucketLifecycleConfiguration(ctx, input)
	if isAPIErrorCode(err, "NoSuchLifecycleConfiguration") {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket lifecycle configuration: %w", err)
	}

	config := &LifecycleConfiguration{
		Rules: make([]LifecycleRule, 0, len(result.Rules)),
	}

	for _, rule := range result.Rules {
		lr := LifecycleRule{
			ID:     getString(rule.ID),
			Status: string(rule.Status),
		}

		if rule.Prefix != nil && *rule.Prefix != "" {
			lr.Prefix = *rule.Prefix
		} else if rule.Filter != nil {
			// v1 summarizes complex filters instead of modeling every predicate.
			lr.Filter = "(complex filter)"
		}

		for _, trans := range rule.Transitions {
			tr := Transition{
				StorageClass: string(trans.StorageClass),
			}
			if trans.Days != nil {
				tr.Days = int(*trans.Days)
			}
			if trans.Date != nil {
				tr.Date = trans.Date.Format("2006-01-02")
			}
			lr.Transitions = append(lr.Transitions, tr)
		}

		if rule.Expiration != nil {
			if rule.Expiration.Days != nil {
				lr.Expiration.Days = int(*rule.Expiration.Days)
			}
			if rule.Expiration.Date != nil {
				lr.Expiration.Date = rule.Expiration.Date.Format("2006-01-02")
			}
		}

		config.Rules = append(config.Rules, lr)
	}

	if len(config.Rules) == 0 {
		return nil, nil
	}

	return config, nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexOfSubstring(s, substr) >= 0)
}

func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func (c *Client) SyncLocalToS3(ctx context.Context, localDir, bucketName, s3Prefix string, progressCallback ProgressCallback) error {
	return filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(localDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		s3Key := filepath.ToSlash(relPath)
		if s3Prefix != "" {
			s3Key = s3Prefix + "/" + s3Key
		}

		shouldUpload := true
		headInput := &s3.HeadObjectInput{
			Bucket: &bucketName,
			Key:    &s3Key,
		}

		headResult, err := c.S3.HeadObject(ctx, headInput)
		if err == nil {
			if headResult.LastModified != nil && !info.ModTime().After(*headResult.LastModified) {
				shouldUpload = false
			}
		}

		if shouldUpload {
			err = c.UploadObjectWithProgress(ctx, bucketName, s3Key, path, progressCallback)
			if err != nil {
				return fmt.Errorf("failed to upload %s: %w", path, err)
			}
		}

		return nil
	})
}

func (c *Client) SyncS3ToLocal(ctx context.Context, bucketName, s3Prefix, localDir string, progressCallback ProgressCallback) error {
	var continuationToken *string

	for {
		result, err := c.ListObjects(ctx, bucketName, s3Prefix, continuationToken)
		if err != nil {
			return err
		}

		for _, obj := range result.Objects {
			if obj.IsFolder {
				continue
			}

			relKey := strings.TrimPrefix(obj.Key, s3Prefix)
			relKey = strings.TrimPrefix(relKey, "/")
			localPath := filepath.Join(localDir, filepath.FromSlash(relKey))

			localDir := filepath.Dir(localPath)
			if err := os.MkdirAll(localDir, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

			shouldDownload := true
			if fileInfo, err := os.Stat(localPath); err == nil {
				objTime, _ := time.Parse("2006-01-02 15:04:05", obj.LastModified)
				if !objTime.After(fileInfo.ModTime()) {
					shouldDownload = false
				}
			}

			if shouldDownload {
				err = c.DownloadObjectWithProgress(ctx, bucketName, obj.Key, localPath, progressCallback)
				if err != nil {
					return fmt.Errorf("failed to download %s: %w", obj.Key, err)
				}
			}
		}

		if !result.IsTruncated {
			break
		}

		continuationToken = result.NextContinuationToken
	}

	return nil
}

func (c *Client) ListObjectVersions(ctx context.Context, bucketName, prefix string) ([]S3ObjectVersion, error) {
	input := &s3.ListObjectVersionsInput{
		Bucket: &bucketName,
	}

	if prefix != "" {
		input.Prefix = &prefix
	}

	result, err := c.S3.ListObjectVersions(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list object versions: %w", err)
	}

	var versions []S3ObjectVersion
	for _, version := range result.Versions {
		v := S3ObjectVersion{
			Key:          getString(version.Key),
			VersionId:    getString(version.VersionId),
			IsLatest:     getBool(version.IsLatest),
			Size:         getInt64(version.Size),
			StorageClass: string(version.StorageClass),
		}

		if version.LastModified != nil {
			v.LastModified = version.LastModified.Format("2006-01-02 15:04:05")
		}

		versions = append(versions, v)
	}

	return versions, nil
}

type S3ObjectVersion struct {
	Key          string
	VersionId    string
	IsLatest     bool
	Size         int64
	LastModified string
	StorageClass string
}

func (c *Client) GetObjectVersion(ctx context.Context, bucketName, key, versionId, localPath string) error {
	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer file.Close()

	input := &s3.GetObjectInput{
		Bucket:    &bucketName,
		Key:       &key,
		VersionId: &versionId,
	}

	result, err := c.S3.GetObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to download object version: %w", err)
	}
	defer result.Body.Close()

	_, err = io.Copy(file, result.Body)
	if err != nil {
		return fmt.Errorf("failed to write to local file: %w", err)
	}

	return nil
}

func (c *Client) DeleteObjectVersion(ctx context.Context, bucketName, key, versionId string) error {
	input := &s3.DeleteObjectInput{
		Bucket:    &bucketName,
		Key:       &key,
		VersionId: &versionId,
	}

	_, err := c.S3.DeleteObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete object version: %w", err)
	}

	return nil
}

func (c *Client) EnableBucketVersioning(ctx context.Context, bucketName string) error {
	enabled := types.BucketVersioningStatusEnabled
	input := &s3.PutBucketVersioningInput{
		Bucket: &bucketName,
		VersioningConfiguration: &types.VersioningConfiguration{
			Status: enabled,
		},
	}

	_, err := c.S3.PutBucketVersioning(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to enable bucket versioning: %w", err)
	}

	return nil
}

func (c *Client) SuspendBucketVersioning(ctx context.Context, bucketName string) error {
	suspended := types.BucketVersioningStatusSuspended
	input := &s3.PutBucketVersioningInput{
		Bucket: &bucketName,
		VersioningConfiguration: &types.VersioningConfiguration{
			Status: suspended,
		},
	}

	_, err := c.S3.PutBucketVersioning(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to suspend bucket versioning: %w", err)
	}

	return nil
}

type S3MultipartUpload struct {
	Key          string
	UploadID     string
	Initiated    string
	StorageClass string
}

// ListMultipartUploads returns nil, nil when the bucket has no in-progress uploads.
func (c *Client) ListMultipartUploads(ctx context.Context, bucketName string) ([]S3MultipartUpload, error) {
	if bucketName == "" {
		return nil, nil
	}

	input := &s3.ListMultipartUploadsInput{
		Bucket: &bucketName,
	}

	result, err := c.S3.ListMultipartUploads(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list multipart uploads: %w", err)
	}

	var uploads []S3MultipartUpload
	for _, upload := range result.Uploads {
		u := S3MultipartUpload{
			Key:      getString(upload.Key),
			UploadID: getString(upload.UploadId),
		}
		if upload.Initiated != nil {
			u.Initiated = upload.Initiated.Format("2006-01-02 15:04:05")
		}
		u.StorageClass = string(upload.StorageClass)
		uploads = append(uploads, u)
	}

	// The API does not guarantee order, so keep the UI stable by sorting newest first.
	for i := 0; i < len(uploads)-1; i++ {
		for j := i + 1; j < len(uploads); j++ {
			if uploads[j].Initiated > uploads[i].Initiated {
				uploads[i], uploads[j] = uploads[j], uploads[i]
			}
		}
	}

	return uploads, nil
}

func (c *Client) AbortMultipartUpload(ctx context.Context, bucketName, key, uploadID string) error {
	if bucketName == "" || key == "" || uploadID == "" {
		return nil
	}

	input := &s3.AbortMultipartUploadInput{
		Bucket:   &bucketName,
		Key:      &key,
		UploadId: &uploadID,
	}

	_, err := c.S3.AbortMultipartUpload(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to abort multipart upload: %w", err)
	}

	return nil
}
