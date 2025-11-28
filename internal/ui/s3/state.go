package s3

import (
	"context"

	"github.com/noelruault/lazyaws/internal/aws"
)

// State contains S3-specific UI and data state.
type State struct {
	Buckets               []aws.Bucket
	FilteredBuckets       []aws.Bucket
	SelectedBucketIndex   int
	CurrentBucket         string
	CurrentPrefix         string
	Objects               []aws.S3Object
	FilteredObjects       []aws.S3Object
	SelectedObjectIndex   int
	NextContinuationToken *string
	IsTruncated           bool
	ObjectDetails         *aws.S3ObjectDetails
	Filter                string
	FilterActive          bool
	PresignedURL          string
	BucketPolicy          string
	BucketVersioning      string
	ShowingInfo           bool
	InfoType              string
	ConfirmDelete         bool
	DeleteTarget          string
	DeleteKey             string
	EditBucket            string
	EditKey               string
	NeedRestore           bool
}

// LoadBuckets fetches S3 buckets.
func LoadBuckets(ctx context.Context, client *aws.Client) ([]aws.Bucket, error) {
	if client == nil {
		return nil, nil
	}
	return client.ListBuckets(ctx)
}

// LoadObjects fetches S3 objects for a bucket/prefix.
func LoadObjects(ctx context.Context, client *aws.Client, bucket, prefix string, continuationToken *string) (*aws.S3ListResult, error) {
	if client == nil || bucket == "" {
		return nil, nil
	}
	return client.ListObjects(ctx, bucket, prefix, continuationToken)
}

// LoadObjectDetails fetches details for a specific object.
func LoadObjectDetails(ctx context.Context, client *aws.Client, bucket, key string) (*aws.S3ObjectDetails, error) {
	if client == nil || bucket == "" || key == "" {
		return nil, nil
	}
	return client.GetObjectDetails(ctx, bucket, key)
}
