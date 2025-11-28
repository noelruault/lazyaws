package ec2

import (
	"fmt"

	"github.com/noelruault/lazyaws/internal/ui/shared"
)

// RenderList renders the EC2 list view (placeholder; to be wired).
func RenderList(st State, vp shared.Viewport) string {
	start, end := shared.GetVisibleRange(len(st.Instances), vp)
	return fmt.Sprintf("EC2 Instances [%d-%d of %d]\n(wiring pending)", start+1, end, len(st.Instances))
}

// RenderDetails renders the EC2 details view (placeholder; to be wired).
func RenderDetails(st State) string {
	if st.InstanceDetails == nil {
		return "EC2 Instance Details\nNo details loaded (wiring pending)"
	}
	return fmt.Sprintf("EC2 Instance Details for %s (wiring pending)", st.InstanceDetails.ID)
}
