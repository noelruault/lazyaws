package ui

import (
	"strings"
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
)

func TestFormatS3ConfigNilPublicAccessBlock(t *testing.T) {
	out := formatS3Config("my-bucket", "Enabled", "eu-west-1", nil, nil, nil, nil, nil, nil, nil, nil, nil, "computing…")
	if !strings.Contains(out, "Block Public Access:\nnot configured") {
		t.Errorf("formatS3Config with nil PAB should report not configured, got: %q", out)
	}
	if !strings.Contains(out, "Size") || !strings.Contains(out, "computing…") {
		t.Errorf("formatS3Config should include the size placeholder, got: %q", out)
	}
	if !strings.Contains(out, "Event Notifications:\nnot configured") {
		t.Errorf("formatS3Config with nil notifications should report not configured, got: %q", out)
	}
}

func TestFormatS3ConfigWithPublicAccessBlock(t *testing.T) {
	pab := &aws.PublicAccessBlock{BlockPublicAcls: true, IgnorePublicAcls: true, BlockPublicPolicy: false, RestrictPublicBuckets: false}
	out := formatS3Config("my-bucket", "Disabled", "us-east-1", pab, nil, nil, nil, nil, nil, nil, nil, nil, "1.0 KiB (3 objects)")
	if !strings.Contains(out, "Block public ACLs: true") {
		t.Errorf("formatS3Config should surface BlockPublicAcls, got: %q", out)
	}
	if !strings.Contains(out, "Block public policy: false") {
		t.Errorf("formatS3Config should surface BlockPublicPolicy, got: %q", out)
	}
}

func TestFormatS3PolicyEmpty(t *testing.T) {
	if got := formatS3Policy(""); got != "no policy\n" {
		t.Errorf("formatS3Policy(\"\") = %q, want no-policy message", got)
	}
}

func TestFormatS3PolicyPrettyPrints(t *testing.T) {
	got := formatS3Policy(`{"Version":"2012-10-17","Statement":[]}`)
	if !strings.Contains(got, "\"Version\": \"2012-10-17\"") {
		t.Errorf("formatS3Policy should pretty-print JSON, got: %q", got)
	}
}

func TestFormatS3PolicyInvalidJSONFallsBack(t *testing.T) {
	if got := formatS3Policy("not json"); got != "not json\n" {
		t.Errorf("formatS3Policy should fall back to raw text on invalid JSON, got: %q", got)
	}
}

func TestFormatS3ConfigMultipartUploads(t *testing.T) {
	uploads := []aws.S3MultipartUpload{
		{Key: "file.zip", UploadID: "abc123", Initiated: "2026-01-15 10:30:00", StorageClass: "STANDARD"},
	}
	out := formatS3Config("my-bucket", "Enabled", "us-east-1", nil, nil, nil, nil, uploads, nil, nil, nil, nil, "1 KiB (1 objects)")
	if !strings.Contains(out, "Multipart Uploads:") {
		t.Errorf("formatS3Config should include Multipart Uploads section, got: %q", out)
	}
	if !strings.Contains(out, "file.zip") {
		t.Errorf("formatS3Config should display the upload key, got: %q", out)
	}
	if !strings.Contains(out, "abc123") {
		t.Errorf("formatS3Config should display the upload ID, got: %q", out)
	}
}

func TestFormatS3ConfigNoMultipartUploads(t *testing.T) {
	out := formatS3Config("my-bucket", "Enabled", "us-east-1", nil, nil, nil, nil, nil, nil, nil, nil, nil, "1 KiB (1 objects)")
	if !strings.Contains(out, "Multipart Uploads:\nnone") {
		t.Errorf("formatS3Config with no uploads should report none, got: %q", out)
	}
}

func TestFormatS3ConfigNilEncryption(t *testing.T) {
	out := formatS3Config("my-bucket", "Enabled", "us-east-1", nil, nil, nil, nil, nil, nil, nil, nil, nil, "1 KiB")
	if !strings.Contains(out, "Server-Side Encryption:\nnot configured (default AWS managed)") {
		t.Errorf("formatS3Config with nil encryption should report not configured, got: %q", out)
	}
}

func TestFormatS3ConfigWithEncryption(t *testing.T) {
	enc := &aws.BucketEncryption{Algorithm: "AES256"}
	out := formatS3Config("my-bucket", "Enabled", "us-east-1", nil, nil, enc, nil, nil, nil, nil, nil, nil, "1 KiB")
	if !strings.Contains(out, "Server-Side Encryption:") {
		t.Errorf("formatS3Config should include Server-Side Encryption section, got: %q", out)
	}
	if !strings.Contains(out, "Algorithm: AES256") {
		t.Errorf("formatS3Config should display encryption algorithm, got: %q", out)
	}
}

func TestFormatS3ConfigWithKMSEncryption(t *testing.T) {
	enc := &aws.BucketEncryption{Algorithm: "aws:kms", KMSKeyID: "arn:aws:kms:us-east-1:123456789012:key/12345678"}
	out := formatS3Config("my-bucket", "Enabled", "us-east-1", nil, nil, enc, nil, nil, nil, nil, nil, nil, "1 KiB")
	if !strings.Contains(out, "Algorithm: aws:kms") {
		t.Errorf("formatS3Config should display KMS algorithm, got: %q", out)
	}
	if !strings.Contains(out, "KMS Key: arn:aws:kms:us-east-1:123456789012:key/12345678") {
		t.Errorf("formatS3Config should display KMS key ID, got: %q", out)
	}
}

func TestFormatS3ConfigNilLifecycle(t *testing.T) {
	out := formatS3Config("my-bucket", "Enabled", "us-east-1", nil, nil, nil, nil, nil, nil, nil, nil, nil, "1 KiB")
	if !strings.Contains(out, "Lifecycle Rules:\nnot configured") {
		t.Errorf("formatS3Config with nil lifecycle should report not configured, got: %q", out)
	}
}

func TestFormatS3ConfigWithLifecycleRules(t *testing.T) {
	lifecycle := &aws.LifecycleConfiguration{
		Rules: []aws.LifecycleRule{
			{
				ID:     "rule1",
				Status: "Enabled",
				Prefix: "logs/",
				Transitions: []aws.Transition{
					{StorageClass: "GLACIER", Days: 30},
					{StorageClass: "DEEP_ARCHIVE", Days: 90},
				},
				Expiration: aws.ExpirationAge{Days: 365},
			},
		},
	}
	out := formatS3Config("my-bucket", "Enabled", "us-east-1", nil, nil, nil, nil, nil, nil, nil, lifecycle, nil, "1 KiB")
	if !strings.Contains(out, "Lifecycle Rules:") {
		t.Errorf("formatS3Config should include Lifecycle Rules section, got: %q", out)
	}
	if !strings.Contains(out, "[rule1]") {
		t.Errorf("formatS3Config should display rule ID, got: %q", out)
	}
	if !strings.Contains(out, "Status: Enabled") {
		t.Errorf("formatS3Config should display rule status, got: %q", out)
	}
	if !strings.Contains(out, "Filter: logs/") {
		t.Errorf("formatS3Config should display rule prefix filter, got: %q", out)
	}
	if !strings.Contains(out, "GLACIER") {
		t.Errorf("formatS3Config should display transition storage class, got: %q", out)
	}
	if !strings.Contains(out, "365 days") {
		t.Errorf("formatS3Config should display expiration days, got: %q", out)
	}
}

func TestFormatS3ConfigWithLifecycleNoPrefix(t *testing.T) {
	lifecycle := &aws.LifecycleConfiguration{
		Rules: []aws.LifecycleRule{
			{
				ID:         "rule2",
				Status:     "Disabled",
				Expiration: aws.ExpirationAge{Days: 180},
			},
		},
	}
	out := formatS3Config("my-bucket", "Enabled", "us-east-1", nil, nil, nil, nil, nil, nil, nil, lifecycle, nil, "1 KiB")
	if !strings.Contains(out, "[rule2]") {
		t.Errorf("formatS3Config should display rule ID, got: %q", out)
	}
	if !strings.Contains(out, "Status: Disabled") {
		t.Errorf("formatS3Config should display disabled status, got: %q", out)
	}
	if !strings.Contains(out, "(all objects)") {
		t.Errorf("formatS3Config should show (all objects) when no filter, got: %q", out)
	}
}

func TestFormatS3ConfigNilTags(t *testing.T) {
	out := formatS3Config("my-bucket", "Enabled", "us-east-1", nil, nil, nil, nil, nil, nil, nil, nil, nil, "1 KiB")
	if !strings.Contains(out, "Tags:\nnone") {
		t.Errorf("formatS3Config with nil tags should report none, got: %q", out)
	}
}

func TestFormatS3ConfigWithTags(t *testing.T) {
	tags := map[string]string{"team": "platform", "env": "prod"}
	out := formatS3Config("my-bucket", "Enabled", "us-east-1", nil, nil, nil, nil, nil, nil, nil, nil, tags, "1 KiB")
	if !strings.Contains(out, "Tags:\n  env: prod\n  team: platform") {
		t.Errorf("formatS3Config should display tags sorted by key, got: %q", out)
	}
}
