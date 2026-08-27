package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/presentation"
	"github.com/noelruault/lazyaws/ui/tasks"
	"github.com/noelruault/lazyaws/ui/utils"
)

func (gui *Gui) getS3Panel() *panels.SideListPanel[*aws.Bucket] {
	return &panels.SideListPanel[*aws.Bucket]{
		ContextState: &panels.ContextState[*aws.Bucket]{
			GetMainTabs: func() []panels.MainTab[*aws.Bucket] {
				return []panels.MainTab[*aws.Bucket]{
					staticOverviewTab(gui, gui.bucketOverview),
					{Key: "config", Title: "Config", Render: gui.renderS3Config},
					{
						Key:    "objects",
						Title:  "Objects",
						Render: gui.renderS3Objects,
						Rows:   func(*aws.Bucket) *panels.MainRows { return gui.s3ObjectRows() },
					},
					{Key: "policy", Title: "Policy", Render: gui.renderS3Policy},
				}
			},
			GetItemContextCacheKey: func(b *aws.Bucket) string {
				return "s3-" + b.Name
			},
		},

		ListPanel: panels.ListPanel[*aws.Bucket]{
			List: panels.NewFilteredList[*aws.Bucket](),
			View: gui.Views.S3,
		},
		NoItemsMessage: "no S3 buckets",
		Gui:            gui.intoInterface(),

		Sort: func(a, b *aws.Bucket) bool {
			return a.Name < b.Name
		},
		GetTableCellsFit: func(b *aws.Bucket) []utils.Cell {
			return presentation.GetBucketDisplayCells(b)
		},
		Weights: func(*aws.Bucket) []int { return presentation.BucketWeights() },
		// A bucket ARN is derivable from the name, but ListBuckets does not answer one and inventing the string here would publish a guess.
		CopyValue: func(b *aws.Bucket) string { return b.Name },
	}
}

func (gui *Gui) loadS3List() error {
	if gui.Client == nil {
		return nil
	}

	gen := gui.Gen

	return gui.WithWaitingStatus("loading s3", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		buckets, err := gui.Client.ListBuckets(ctx)
		if err != nil {
			return err
		}
		if gen != gui.Gen {
			return nil
		}

		rows := make([]*aws.Bucket, len(buckets))
		for i := range buckets {
			rows[i] = &buckets[i]
		}
		gui.Panels.S3.SetItemsKeepSelection(rows, s3SelectionKey)
		return gui.Panels.S3.RerenderList()
	})
}

// s3SelectionKey identifies a bucket across reloads; bucket names are globally unique.
func s3SelectionKey(bucket *aws.Bucket) string { return bucket.Name }

// bucketOverview re-lays the Config tab's data, minus the size: GetBucketSize scans every object in the bucket, and the overview is the tab a selection opens on.
func (gui *Gui) bucketOverview(ctx context.Context, bucket *aws.Bucket, width int) string {
	if gui.Client == nil {
		return overviewUnavailable("bucket")
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	return presentation.FormatBucketOverview(bucket, gui.Client.GetBucketOverview(fetchCtx, bucket.Name), width, time.Now())
}

// renderS3Config defers the full bucket scan so slow size calculation cannot block other metadata.
func (gui *Gui) renderS3Config(bucket *aws.Bucket) tasks.TaskFunc {
	name := bucket.Name
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		versioning, err := gui.Client.GetBucketVersioning(fetchCtx, name)
		if gen != gui.Gen {
			return
		}
		if err != nil {
			gui.RenderStringMain("error loading bucket config: " + err.Error())
			return
		}

		// Everything past versioning is best-effort: an error shouldn't blank out the info we already have.
		region, regionErr := gui.Client.GetBucketRegion(fetchCtx, name)
		if regionErr != nil {
			region = "unknown"
		}
		pab, _ := gui.Client.GetBucketPublicAccessBlock(fetchCtx, name)
		notifications, _ := gui.Client.GetBucketNotificationConfiguration(fetchCtx, name)
		encryption, _ := gui.Client.GetBucketEncryption(fetchCtx, name)
		objectLock, _ := gui.Client.GetBucketObjectLockConfiguration(fetchCtx, name)
		uploads, _ := gui.Client.ListMultipartUploads(fetchCtx, name)
		logging, _ := gui.Client.GetBucketLogging(fetchCtx, name)
		replication, _ := gui.Client.GetBucketReplication(fetchCtx, name)
		lifecycle, _ := gui.Client.GetBucketLifecycleConfiguration(fetchCtx, name)
		tags, _ := gui.Client.GetBucketTagging(fetchCtx, name)
		if gen != gui.Gen {
			return
		}
		gui.RenderStringMain(formatS3Config(name, versioning, region, pab, notifications, encryption, objectLock, uploads, logging, replication, lifecycle, tags, "computing…"))

		sizeCtx, sizeCancel := context.WithTimeout(ctx, 60*time.Second)
		defer sizeCancel()
		size, count, sizeErr := gui.Client.GetBucketSize(sizeCtx, name)
		if gen != gui.Gen {
			return
		}
		sizeStr := "unknown"
		if sizeErr == nil {
			sizeStr = fmt.Sprintf("%s (%d objects)", formatByteCount(float64(size)), count)
		}
		gui.RenderStringMain(formatS3Config(name, versioning, region, pab, notifications, encryption, objectLock, uploads, logging, replication, lifecycle, tags, sizeStr))
	}})
}

func formatS3Config(name, versioning, region string, pab *aws.PublicAccessBlock, notifications *aws.NotificationConfig, encryption *aws.BucketEncryption, objectLock *aws.ObjectLockConfiguration, uploads []aws.S3MultipartUpload, logging *aws.BucketLogging, replication *aws.BucketReplication, lifecycle *aws.LifecycleConfiguration, tags map[string]string, size string) string {
	out := utils.FormatMap(0, map[string]string{
		"Name":       name,
		"Region":     region,
		"Versioning": versioning,
		"Size":       size,
	})

	out += "\nBlock Public Access:\n"
	if pab == nil {
		out += "not configured\n"
	} else {
		out += fmt.Sprintf("  Block public ACLs: %v\n  Ignore public ACLs: %v\n  Block public policy: %v\n  Restrict public buckets: %v\n",
			pab.BlockPublicAcls, pab.IgnorePublicAcls, pab.BlockPublicPolicy, pab.RestrictPublicBuckets)
	}

	out += "\nServer-Side Encryption:\n"
	if encryption == nil {
		out += "not configured (default AWS managed)\n"
	} else {
		out += fmt.Sprintf("  Algorithm: %s\n", encryption.Algorithm)
		if encryption.KMSKeyID != "" {
			out += fmt.Sprintf("  KMS Key: %s\n", encryption.KMSKeyID)
		}
	}

	out += "\nObject Lock:\n"
	if objectLock == nil || !objectLock.Enabled {
		out += "not configured\n"
	} else {
		out += "  Enabled: yes\n"
		if objectLock.DefaultRetentionMode != "" {
			out += fmt.Sprintf("  Default Retention Mode: %s\n", objectLock.DefaultRetentionMode)
			if objectLock.DefaultRetentionDays > 0 {
				out += fmt.Sprintf("  Default Retention Days: %d\n", objectLock.DefaultRetentionDays)
			}
		} else {
			out += "  Default Retention: none\n"
		}
	}

	out += "\nEvent Notifications:\n"
	if notifications == nil || (len(notifications.LambdaFunctions) == 0 && len(notifications.Topics) == 0 && len(notifications.Queues) == 0) {
		out += "not configured\n"
	} else {
		if len(notifications.LambdaFunctions) > 0 {
			out += "  Lambda Functions:\n"
			for _, fn := range notifications.LambdaFunctions {
				out += fmt.Sprintf("    [%s] %s\n", fn.ID, fn.Function)
				out += fmt.Sprintf("      Events: %v\n", fn.Events)
				if fn.Filter != "" {
					out += fmt.Sprintf("      Filter: %s\n", fn.Filter)
				}
			}
		}
		if len(notifications.Topics) > 0 {
			out += "  SNS Topics:\n"
			for _, topic := range notifications.Topics {
				out += fmt.Sprintf("    [%s] %s\n", topic.ID, topic.Topic)
				out += fmt.Sprintf("      Events: %v\n", topic.Events)
				if topic.Filter != "" {
					out += fmt.Sprintf("      Filter: %s\n", topic.Filter)
				}
			}
		}
		if len(notifications.Queues) > 0 {
			out += "  SQS Queues:\n"
			for _, queue := range notifications.Queues {
				out += fmt.Sprintf("    [%s] %s\n", queue.ID, queue.Queue)
				out += fmt.Sprintf("      Events: %v\n", queue.Events)
				if queue.Filter != "" {
					out += fmt.Sprintf("      Filter: %s\n", queue.Filter)
				}
			}
		}
	}

	out += "\nMultipart Uploads:\n"
	if len(uploads) == 0 {
		out += "none\n"
	} else {
		for _, u := range uploads {
			out += fmt.Sprintf("  %s (ID: %s)\n", u.Key, u.UploadID)
			out += fmt.Sprintf("    Initiated: %s\n", u.Initiated)
			out += fmt.Sprintf("    Storage Class: %s\n", u.StorageClass)
		}
	}

	out += "\nServer Access Logging:\n"
	if logging == nil || logging.TargetBucket == "" {
		out += "disabled\n"
	} else {
		out += fmt.Sprintf("  Target Bucket: %s\n", logging.TargetBucket)
		if logging.TargetPrefix != "" {
			out += fmt.Sprintf("  Target Prefix: %s\n", logging.TargetPrefix)
		}
	}

	out += "\nReplication Rules:\n"
	if replication == nil || len(replication.Rules) == 0 {
		out += "not configured\n"
	} else {
		for _, rule := range replication.Rules {
			out += fmt.Sprintf("  [%s] Status: %s\n", rule.ID, rule.Status)
			out += fmt.Sprintf("    Destination: %s\n", rule.DestinationBucket)
			out += fmt.Sprintf("    Region: %s\n", rule.DestinationRegion)
			out += fmt.Sprintf("    Type: %s\n", rule.ReplicationType)
		}
	}

	out += "\nLifecycle Rules:\n"
	if lifecycle == nil || len(lifecycle.Rules) == 0 {
		out += "not configured\n"
	} else {
		for _, rule := range lifecycle.Rules {
			out += fmt.Sprintf("  [%s] Status: %s\n", rule.ID, rule.Status)
			filter := rule.Prefix
			if filter == "" && rule.Filter != "" {
				filter = rule.Filter
			}
			if filter == "" {
				filter = "(all objects)"
			}
			out += fmt.Sprintf("    Filter: %s\n", filter)
			if len(rule.Transitions) > 0 {
				out += "    Transitions:\n"
				for _, trans := range rule.Transitions {
					if trans.Days > 0 {
						out += fmt.Sprintf("      → %s (after %d days)\n", trans.StorageClass, trans.Days)
					} else if trans.Date != "" {
						out += fmt.Sprintf("      → %s (on %s)\n", trans.StorageClass, trans.Date)
					}
				}
			}
			if rule.Expiration.Days > 0 {
				out += fmt.Sprintf("    Expiration: %d days\n", rule.Expiration.Days)
			} else if rule.Expiration.Date != "" {
				out += fmt.Sprintf("    Expiration: %s\n", rule.Expiration.Date)
			}
		}
	}

	out += "\nTags:\n"
	if len(tags) == 0 {
		out += "none\n"
	} else {
		keys := make([]string, 0, len(tags))
		for k := range tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out += fmt.Sprintf("  %s: %s\n", k, tags[k])
		}
	}

	return out
}

func (gui *Gui) renderS3Policy(bucket *aws.Bucket) tasks.TaskFunc {
	name := bucket.Name
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		policy, err := gui.Client.GetBucketPolicy(fetchCtx, name)
		if gen != gui.Gen {
			return
		}
		if err != nil {
			gui.RenderStringMain("error loading bucket policy: " + err.Error())
			return
		}
		gui.RenderStringMain(formatS3Policy(policy))
	}})
}

func formatS3Policy(policy string) string {
	if policy == "" {
		return "no policy\n"
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(policy), "", "  "); err != nil {
		return policy + "\n"
	}
	return buf.String() + "\n"
}
