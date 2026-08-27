package aws

import (
	"context"
	"strings"
	"testing"
)

// A client with no SDK clients is the one state a test can drive without an interface seam, and it is the state that proves the fan-out's contract: every section fails, every failure is reported, and none of them takes the pane down with it.
// It is also what proves the guard is inside the fan-out: the S3 subresource calls dereference c.S3 directly, so without it this test panics in a goroutine rather than failing.
func TestGetBucketOverviewReportsEverySectionThatFailed(t *testing.T) {
	overview := (&Client{}).GetBucketOverview(context.Background(), "app-artifacts")

	if overview == nil {
		t.Fatal("GetBucketOverview() = nil, want an overview even when every section failed")
	}

	for _, section := range []string{
		SectionRegion, SectionVersioning, SectionPublicAccess, SectionEncryption, SectionObjectLock,
		SectionLifecycle, SectionReplication, SectionLogging, SectionNotifications, SectionPolicy, SectionTags,
	} {
		err := overview.Err(section)
		if err == nil {
			t.Errorf("Err(%q) = nil, want the failed fetch to be reported", section)
			continue
		}
		if !strings.Contains(err.Error(), "S3 client not initialized") {
			t.Errorf("Err(%q) = %v, want the nil-client guard rather than a panic further in", section, err)
		}
	}

	if overview.PublicAccess != nil || overview.Encryption != nil || overview.ObjectLock != nil ||
		overview.Lifecycle != nil || overview.Replication != nil || overview.Logging != nil || overview.Notifications != nil {
		t.Error("a failed section should leave its field nil rather than a zero value the formatter would render as data")
	}
	if overview.PolicyPresent {
		t.Error("a failed policy read reported a policy as attached")
	}
}

// The overview keeps whether a policy exists, not the document, and inverted this reads as an attached policy on a bucket governed by none.
func TestBucketPolicyAttached(t *testing.T) {
	if BucketPolicyAttached("") {
		t.Error("an empty policy response is NoSuchBucketPolicy, which is no policy")
	}
	if !BucketPolicyAttached(`{"Version":"2012-10-17","Statement":[]}`) {
		t.Error("a policy document was reported as no policy")
	}
}

// Err answers per section, which is what lets one formatter section render "unavailable" while its neighbours render data.
func TestBucketOverviewErrIsPerSection(t *testing.T) {
	overview := &BucketOverview{Errs: map[string]error{SectionPolicy: context.Canceled}}

	if overview.Err(SectionPolicy) == nil {
		t.Error("Err() did not report the section that failed")
	}
	if overview.Err(SectionEncryption) != nil {
		t.Error("Err() reported a section that did not fail")
	}
}
