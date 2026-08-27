package aws

import (
	"context"
	"errors"
)

// The BucketOverview.Errs keys. Every bucket subresource is a separate S3 call answering a separate question, so a denied GetBucketPolicy has to be reportable in the policy line without taking the encryption line down with it.
const (
	SectionRegion        = "region"
	SectionVersioning    = "versioning"
	SectionPublicAccess  = "public-access"
	SectionEncryption    = "encryption"
	SectionObjectLock    = "object-lock"
	SectionLifecycle     = "lifecycle"
	SectionReplication   = "replication"
	SectionLogging       = "logging"
	SectionNotifications = "notifications"
	SectionPolicy        = "policy"
	SectionTags          = "tags"
)

// BucketOverview aggregates the bucket configuration the Config tab fetches, minus the size.
// GetBucketSize is a full ListObjectsV2 scan of the bucket, so it stays on demand: an overview is the tab a bucket opens on, and it must not put an unbounded object listing behind a selection.
// A nil pointer field means the feature is not configured, which is what every one of these calls reports an absence as; a failure lands in Errs instead.
type BucketOverview struct {
	Region        string
	Versioning    string
	PublicAccess  *PublicAccessBlock
	Encryption    *BucketEncryption
	ObjectLock    *ObjectLockConfiguration
	Lifecycle     *LifecycleConfiguration
	Replication   *BucketReplication
	Logging       *BucketLogging
	Notifications *NotificationConfig
	Tags          map[string]string

	// PolicyPresent, not the policy: the document itself is the Policy tab's job, and an overview only answers whether one is attached.
	PolicyPresent bool

	Errs map[string]error
}

// Err reports the fetch error for a section, if that section failed.
func (o *BucketOverview) Err(section string) error {
	return o.Errs[section]
}

// GetBucketOverview fetches the bucket's configuration concurrently and always returns an overview, never an error.
// A section that failed is reported through Errs so the sections that succeeded still render: on a read-only role, a denied GetBucketPolicy is routine and must cost one line rather than the pane.
func (c *Client) GetBucketOverview(ctx context.Context, name string) *BucketOverview {
	overview := &BucketOverview{Errs: map[string]error{}}
	sections := newSectionFetcher(overview.Errs)

	sections.fetch(SectionRegion, c.bucketSection(func() (err error) {
		overview.Region, err = c.GetBucketRegion(ctx, name)
		return err
	}))
	sections.fetch(SectionVersioning, c.bucketSection(func() (err error) {
		overview.Versioning, err = c.GetBucketVersioning(ctx, name)
		return err
	}))
	sections.fetch(SectionPublicAccess, c.bucketSection(func() (err error) {
		overview.PublicAccess, err = c.GetBucketPublicAccessBlock(ctx, name)
		return err
	}))
	sections.fetch(SectionEncryption, c.bucketSection(func() (err error) {
		overview.Encryption, err = c.GetBucketEncryption(ctx, name)
		return err
	}))
	sections.fetch(SectionObjectLock, c.bucketSection(func() (err error) {
		overview.ObjectLock, err = c.GetBucketObjectLockConfiguration(ctx, name)
		return err
	}))
	sections.fetch(SectionLifecycle, c.bucketSection(func() (err error) {
		overview.Lifecycle, err = c.GetBucketLifecycleConfiguration(ctx, name)
		return err
	}))
	sections.fetch(SectionReplication, c.bucketSection(func() (err error) {
		overview.Replication, err = c.GetBucketReplication(ctx, name)
		return err
	}))
	sections.fetch(SectionLogging, c.bucketSection(func() (err error) {
		overview.Logging, err = c.GetBucketLogging(ctx, name)
		return err
	}))
	sections.fetch(SectionNotifications, c.bucketSection(func() (err error) {
		overview.Notifications, err = c.GetBucketNotificationConfiguration(ctx, name)
		return err
	}))
	sections.fetch(SectionPolicy, c.bucketSection(func() error {
		policy, err := c.GetBucketPolicy(ctx, name)
		overview.PolicyPresent = policy != ""
		return err
	}))
	sections.fetch(SectionTags, c.bucketSection(func() (err error) {
		overview.Tags, err = c.GetBucketTagging(ctx, name)
		return err
	}))

	sections.wait()

	return overview
}

// bucketSection runs a fetch behind the nil-client check every S3 subresource call needs, because none of them carries its own.
// Guarding inside the fan-out rather than ahead of it is what keeps the failure per section: a client with no S3 reports eleven failed sections like any other outage, instead of one error standing in for the pane.
func (c *Client) bucketSection(run func() error) func() error {
	return func() error {
		if c.S3 == nil {
			return errors.New("S3 client not initialized")
		}

		return run()
	}
}
