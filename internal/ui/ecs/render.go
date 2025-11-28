package ecs

import (
	"fmt"

	"github.com/noelruault/lazyaws/internal/ui/shared"
)

// RenderClusters provides a placeholder list view for ECS clusters.
func RenderClusters(st State, vp shared.Viewport) string {
	start, end := shared.GetVisibleRange(len(st.Clusters), vp)
	return fmt.Sprintf("ECS Clusters [%d-%d of %d]\n(wiring pending)", start+1, end, len(st.Clusters))
}
