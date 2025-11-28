package s3

import (
	"fmt"

	"github.com/fuziontech/lazyaws/internal/ui/shared"
)

// RenderBuckets renders the bucket list (placeholder; to be wired).
func RenderBuckets(st State, vp shared.Viewport) string {
	start, end := shared.GetVisibleRange(len(st.Buckets), vp)
	return fmt.Sprintf("S3 Buckets [%d-%d of %d]\n(wiring pending)", start+1, end, len(st.Buckets))
}

// RenderObjects renders the object list (placeholder; to be wired).
func RenderObjects(st State, vp shared.Viewport) string {
	start, end := shared.GetVisibleRange(len(st.Objects), vp)
	return fmt.Sprintf("S3 Objects [%d-%d of %d]\n(wiring pending)", start+1, end, len(st.Objects))
}

// RenderObjectDetails renders object details (placeholder; to be wired).
func RenderObjectDetails(st State) string {
	if st.ObjectDetails == nil {
		return "S3 Object Details\nNo details loaded (wiring pending)"
	}
	return fmt.Sprintf("S3 Object Details for %s (wiring pending)", st.ObjectDetails.Key)
}
