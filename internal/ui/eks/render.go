package eks

import (
	"fmt"

	"github.com/fuziontech/lazyaws/internal/ui/shared"
)

// RenderClusters renders the EKS cluster list (placeholder; to be wired).
func RenderClusters(st State, vp shared.Viewport) string {
	start, end := shared.GetVisibleRange(len(st.Clusters), vp)
	return fmt.Sprintf("EKS Clusters [%d-%d of %d]\n(wiring pending)", start+1, end, len(st.Clusters))
}

// RenderDetails renders the EKS cluster details (placeholder; to be wired).
func RenderDetails(st State) string {
	if st.Details == nil {
		return "EKS Cluster Details\nNo details loaded (wiring pending)"
	}
	return fmt.Sprintf("EKS Cluster Details for %s (wiring pending)", st.Details.Name)
}
