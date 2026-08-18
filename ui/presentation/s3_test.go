package presentation

import (
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
)

func TestGetBucketDisplayStrings(t *testing.T) {
	b := &aws.Bucket{Name: "my-bucket", Region: "eu-west-1", CreationDate: "2026-07-10 00:00:00"}
	got := GetBucketDisplayStrings(b)
	want := []string{"my-bucket", "eu-west-1", "2026-07-10 00:00:00"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cell %d = %q, want %q (row %v)", i, got[i], want[i], got)
		}
	}
}
