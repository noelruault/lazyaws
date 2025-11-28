package ecr

import (
	"fmt"

	"github.com/noelruault/lazyaws/internal/ui/shared"
)

// RenderRepositories provides a placeholder view for ECR repositories.
func RenderRepositories(st State, vp shared.Viewport) string {
	start, end := shared.GetVisibleRange(len(st.Repositories), vp)
	return fmt.Sprintf("ECR Repositories [%d-%d of %d]\n(wiring pending)", start+1, end, len(st.Repositories))
}
