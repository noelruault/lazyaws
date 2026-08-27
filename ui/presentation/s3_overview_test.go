package presentation

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// kvPadding is the run of spaces kvBlock inserts to line a block's values up, which is as wide as the widest label in THAT block and so changes when a section drops a row.
var kvPadding = regexp.MustCompile(` {2,}`)

// plainBucket renders the overview as the words it chose: escapes stripped and the alignment padding collapsed, since neither is what these states are about.
func plainBucket(b *aws.Bucket, o *aws.BucketOverview, width int) string {
	return kvPadding.ReplaceAllString(utils.Decolorise(FormatBucketOverview(b, o, width, overviewNow)), " ")
}

func overviewBucket() *aws.Bucket {
	return &aws.Bucket{Name: "app-artifacts", Region: "-", CreationDate: "2026-08-20 09:00:00"}
}

// fullBucketOverview answers every fetch, so a test that removes one thing is testing that one thing.
func fullBucketOverview() *aws.BucketOverview {
	return &aws.BucketOverview{
		Region:       "eu-west-1",
		Versioning:   "Enabled",
		PublicAccess: &aws.PublicAccessBlock{BlockPublicAcls: true, IgnorePublicAcls: true, BlockPublicPolicy: true, RestrictPublicBuckets: true},
		Encryption:   &aws.BucketEncryption{Algorithm: "aws:kms", KMSKeyID: "arn:aws:kms:eu-west-1:123456789012:key/2f7c"},
		ObjectLock:   &aws.ObjectLockConfiguration{Enabled: true, DefaultRetentionMode: "GOVERNANCE", DefaultRetentionDays: 30},
		Lifecycle: &aws.LifecycleConfiguration{Rules: []aws.LifecycleRule{
			{ID: "archive", Status: "Enabled", Prefix: "builds/", Transitions: []aws.Transition{{StorageClass: "GLACIER", Days: 30}}, Expiration: aws.ExpirationAge{Days: 365}},
			{ID: "drop-temp", Status: "Disabled", Expiration: aws.ExpirationAge{Days: 7}},
		}},
		Replication: &aws.BucketReplication{Rules: []aws.ReplicationRule{
			{ID: "dr", Status: "Enabled", DestinationBucket: "app-artifacts-dr", DestinationRegion: "eu-central-1"},
		}},
		Logging:       &aws.BucketLogging{TargetBucket: "app-access-logs", TargetPrefix: "artifacts/"},
		Notifications: &aws.NotificationConfig{LambdaFunctions: []aws.LambdaNotification{{ID: "scan"}}, Queues: []aws.SQSNotification{{ID: "index"}}},
		Tags:          map[string]string{"Env": "prod", "Owner": "platform"},
		PolicyPresent: true,
		Errs:          map[string]error{},
	}
}

// emptyBucketOverview answers with the absence S3 actually reports: a nil subresource per unconfigured feature, an empty policy, and no tags.
func emptyBucketOverview() *aws.BucketOverview {
	return &aws.BucketOverview{Region: "eu-west-1", Versioning: "Disabled", Errs: map[string]error{}}
}

func TestBucketOverviewRendersEverySection(t *testing.T) {
	got := plainBucket(overviewBucket(), fullBucketOverview(), stackedWidth)

	for _, want := range []string{
		"Bucket", "app-artifacts", "Public access blocked", "eu-west-1",
		"Security", "Block ACLs:", "Restrict public:", "aws:kms", "attached, shown on the Policy tab",
		"Data management", "Enabled", "GOVERNANCE", "30d default",
		"Lifecycle: 2 rules", "[archive] Enabled · builds/ · → GLACIER 30d · expire 365d", "[drop-temp] Disabled · all objects · expire 7d",
		"Replication: 1 rule", "[dr] Enabled → app-artifacts-dr (eu-central-1)",
		"Access", "app-access-logs/artifacts/", "1 lambda, 1 queue",
		"Tags", "Env: prod", "Owner: platform",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview is missing %q\n%s", want, got)
		}
	}
}

// The creation stamp carries its age, and the list maps it with a space rather than as RFC3339, so the parse has to match that layout or every bucket reads "unknown".
func TestBucketOverviewDatesTheCreationStamp(t *testing.T) {
	got := plainBucket(overviewBucket(), fullBucketOverview(), stackedWidth)

	if want := "created 2026-08-20 09:00:00 (7d ago)"; !strings.Contains(got, want) {
		t.Errorf("overview is missing %q\n%s", want, got)
	}
}

// An unparseable stamp degrades to the stamp itself: a wrong age is worse than no age.
func TestBucketCreatedSurvivesAnUnparseableStamp(t *testing.T) {
	bucket := overviewBucket()
	bucket.CreationDate = "not a timestamp"
	if got := bucketCreated(bucket, overviewNow); got != "created not a timestamp" {
		t.Errorf("bucketCreated(unparseable) = %q, want the stamp passed through", got)
	}

	bucket.CreationDate = ""
	if got := bucketCreated(bucket, overviewNow); got != "" {
		t.Errorf("bucketCreated(\"\") = %q, want it dropped from the header", got)
	}
}

// Every absence S3 reports as a nil subresource says what it is, rather than leaving a heading with nothing under it.
func TestBucketOverviewStatesEveryAbsence(t *testing.T) {
	got := plainBucket(overviewBucket(), emptyBucketOverview(), stackedWidth)

	for _, want := range []string{
		"Public access: no block configuration",
		"AES256 (S3 default, not configured on the bucket)",
		"Bucket policy: none",
		"Object lock: not configured",
		"Lifecycle: none",
		"Replication: none",
		"Access logging: disabled",
		"Notifications: none",
		"Tags\nnone",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview is missing %q\n%s", want, got)
		}
	}
}

// A bucket with no block configuration is not a bucket whose block configuration could not be read: the first is the commonest state there is, and reporting it as a failure sends someone to fix an IAM policy that is fine.
func TestBucketOverviewSeparatesAnAbsentBlockFromAFailedRead(t *testing.T) {
	absent := plainBucket(overviewBucket(), emptyBucketOverview(), stackedWidth)
	if !strings.Contains(absent, "Public access: no block configuration") {
		t.Errorf("an absent public access block should read as no configuration\n%s", absent)
	}
	if strings.Contains(absent, "unavailable") {
		t.Errorf("an absent public access block should not read as a failed fetch\n%s", absent)
	}

	// Built from the FULL fixture, which carries a block AND the error: today the fetch nils the block whenever it fails, so an empty fixture cannot tell the error guard from the nil check beside it.
	o := fullBucketOverview()
	o.Errs[aws.SectionPublicAccess] = errors.New("AccessDenied")
	failed := plainBucket(overviewBucket(), o, stackedWidth)
	if !strings.Contains(failed, "Public access: unavailable: AccessDenied") {
		t.Errorf("a failed public access read should say so\n%s", failed)
	}
	// The four flag rows come off the same response, so a failed read must not print four settings it could not read.
	if strings.Contains(failed, "Block ACLs") {
		t.Errorf("a failed public access read should not render its flag rows\n%s", failed)
	}
}

// Every fetch is its own S3 call on a read-only role, so one denial costs its own line and leaves the rest of the pane standing.
func TestBucketOverviewFieldsFailIndependently(t *testing.T) {
	tests := []struct {
		section string
		want    string
	}{
		{aws.SectionVersioning, "Versioning: unavailable: boom"},
		{aws.SectionEncryption, "Encryption: unavailable: boom"},
		{aws.SectionObjectLock, "Object lock: unavailable: boom"},
		{aws.SectionPolicy, "Bucket policy: unavailable: boom"},
		{aws.SectionLifecycle, "Lifecycle: unavailable: boom"},
		{aws.SectionReplication, "Replication: unavailable: boom"},
		{aws.SectionLogging, "Access logging: unavailable: boom"},
		{aws.SectionNotifications, "Notifications: unavailable: boom"},
		{aws.SectionTags, "Tags\nunavailable: boom"},
	}

	for _, test := range tests {
		t.Run(test.section, func(t *testing.T) {
			o := fullBucketOverview()
			o.Errs[test.section] = errors.New("boom")
			got := plainBucket(overviewBucket(), o, stackedWidth)

			if !strings.Contains(got, test.want) {
				t.Errorf("overview is missing %q\n%s", test.want, got)
			}
			// The pane survives: the bucket is still identified and the sections that answered are still rendered.
			if !strings.Contains(got, "app-artifacts") || !strings.Contains(got, "Security") {
				t.Errorf("a failed %s took the pane down with it\n%s", test.section, got)
			}
			if count := strings.Count(got, "unavailable"); count != 1 {
				t.Errorf("a failed %s made %d fields unavailable, want 1\n%s", test.section, count, got)
			}
		})
	}
}

// A failed region lookup cannot leave the header claiming a region, and the list row's placeholder is not one either.
func TestBucketOverviewReportsAFailedRegionInTheHeader(t *testing.T) {
	o := fullBucketOverview()
	o.Errs[aws.SectionRegion] = errors.New("AccessDenied")

	if got := plainBucket(overviewBucket(), o, stackedWidth); !strings.Contains(got, "region unavailable") {
		t.Errorf("overview does not report the failed region lookup\n%s", got)
	}
}

// The badge is the one thing read at a glance, and every state it can take has to be a different word.
func TestBucketExposureBadge(t *testing.T) {
	forceColor(t)

	all := &aws.PublicAccessBlock{BlockPublicAcls: true, IgnorePublicAcls: true, BlockPublicPolicy: true, RestrictPublicBuckets: true}
	tests := []struct {
		name  string
		block *aws.PublicAccessBlock
		err   error
		want  string
	}{
		{"all four", all, nil, "Public access blocked"},
		{"two of four", &aws.PublicAccessBlock{BlockPublicAcls: true, BlockPublicPolicy: true}, nil, "Public access partly blocked (2/4)"},
		{"none of four", &aws.PublicAccessBlock{}, nil, "Public access not blocked"},
		{"no configuration", nil, nil, "Public access not blocked"},
		{"failed read", all, errors.New("AccessDenied"), "Public access unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			o := &aws.BucketOverview{PublicAccess: test.block, Errs: map[string]error{}}
			if test.err != nil {
				o.Errs[aws.SectionPublicAccess] = test.err
			}

			if got := utils.Decolorise(bucketExposureBadge(o)); got != test.want {
				t.Errorf("badge = %q, want %q", got, test.want)
			}
		})
	}
}

// Go randomizes map iteration, so an unsorted tag block reorders itself on every re-render of the same bucket.
func TestBucketOverviewSortsTags(t *testing.T) {
	o := emptyBucketOverview()
	// Six keys, not three: with three, a dropped sort still lands in order once every six runs, and a mutant that survives one run in six is a test nobody can trust.
	o.Tags = map[string]string{"zeta": "6", "mu": "4", "alpha": "1", "tau": "5", "beta": "2", "kappa": "3"}

	got := plainBucket(overviewBucket(), o, stackedWidth)
	if want := "alpha: 1\nbeta: 2\nkappa: 3\nmu: 4\ntau: 5\nzeta: 6"; !strings.Contains(got, want) {
		t.Errorf("tags are not sorted, want %q\n%s", want, got)
	}
}

// A bucket can carry more lifecycle rules than a pane has lines, and silently showing the first few would read as the whole policy.
func TestBucketOverviewCapsTheRuleListAndSaysSo(t *testing.T) {
	o := emptyBucketOverview()
	for i := range bucketRulesShown + 3 {
		o.Lifecycle = &aws.LifecycleConfiguration{Rules: append(ruleSet(o.Lifecycle), aws.LifecycleRule{ID: "rule-" + string(rune('a'+i)), Status: "Enabled"})}
	}

	got := plainBucket(overviewBucket(), o, stackedWidth)
	if want := "Lifecycle: 8 rules"; !strings.Contains(got, want) {
		t.Errorf("overview is missing %q\n%s", want, got)
	}
	if want := "(3 more on the Config tab)"; !strings.Contains(got, want) {
		t.Errorf("overview is missing %q\n%s", want, got)
	}
	if strings.Contains(got, "rule-f") {
		t.Errorf("overview rendered a rule past the cap\n%s", got)
	}
}

func ruleSet(c *aws.LifecycleConfiguration) []aws.LifecycleRule {
	if c == nil {
		return nil
	}

	return c.Rules
}

// Wrapping is off on an overview, so a line over its budget runs off the pane rather than folding.
func TestBucketOverviewNeverExceedsTheWidth(t *testing.T) {
	forceColor(t)

	bucket := overviewBucket()
	// A long name in the header, which Columns never measures because it spans the full width, and a lifecycle rule that runs past any column.
	bucket.Name = "a-very-long-bucket-name-nobody-should-have-but-someone-in-eu-west-1-does"
	o := fullBucketOverview()
	o.Lifecycle.Rules[0].Prefix = "builds/very/deep/prefix/that/keeps/going/until/it/runs/off/the/pane/entirely/"

	for width := 40; width <= 220; width++ {
		for _, line := range strings.Split(FormatBucketOverview(bucket, o, width, overviewNow), "\n") {
			if got := runewidth.StringWidth(utils.Decolorise(line)); got > width {
				t.Fatalf("at width %d a line is %d cells wide: %q", width, got, utils.Decolorise(line))
			}
		}
	}
}
