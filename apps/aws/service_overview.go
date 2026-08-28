package aws

import (
	"context"
	"time"
)

// SectionImage is the ECSServiceOverview.Errs key for the image resolution, which is its own fetch rather than a field of another one.
// SectionMetrics is shared with the instance and cluster overviews: the key names the fetch that failed, and each pane has exactly one metrics fetch.
const (
	SectionImage   = "image"
	SectionScaling = "scaling"
)

// ECSServiceOverview aggregates the fetches a service pane cannot answer from the list row it already holds.
// Deployments, networking, counts and events all arrive with DescribeServices, so they are read off the service itself and cannot fail independently of it.
type ECSServiceOverview struct {
	Metrics *ECSServiceMetrics
	Image   ECSServiceImage
	// Scaling is nil for a service with no Application Auto Scaling registered, which is an answer and not a failure.
	Scaling *ECSServiceAutoScaling

	Errs map[string]error
}

// Err reports the fetch error for a section, if that section failed.
func (o *ECSServiceOverview) Err(section string) error {
	return o.Errs[section]
}

// GetECSServiceOverview fetches the service's sections concurrently and always returns an overview, never an error.
// A section that failed is reported through Errs so the sections that succeeded still render: one denied permission degrades one block instead of blanking the pane.
// metricsMaxAge puts the metrics section on its own slower tier, for the reason GetInstanceOverview documents; 0 means the reading taken for this selection is reused for as long as it is selected.
func (c *Client) GetECSServiceOverview(ctx context.Context, s *ECSService, metricsMaxAge time.Duration) *ECSServiceOverview {
	overview := &ECSServiceOverview{Errs: map[string]error{}}
	if s == nil {
		return overview
	}

	sections := newSectionFetcher(overview.Errs)

	sections.fetch(SectionMetrics, func() (err error) {
		// Keyed on cluster AND service: a service name is unique only within its cluster, and two clusters running the same service name is the normal shape of a blue/green or per-environment layout.
		overview.Metrics, err = memoized(&c.serviceMetrics, s.Cluster+"/"+s.Name, metricsMaxAge, func() (*ECSServiceMetrics, error) {
			return c.GetECSServiceMetrics(ctx, s.Cluster, s.Name)
		})
		return err
	})
	// The image is the one section that costs a task listing, and spec.md's hard requirement is that an ECS view shows what a deployment is actually running.
	sections.fetch(SectionImage, func() (err error) {
		overview.Image, err = c.ResolveECSServiceImage(ctx, s)
		return err
	})
	sections.fetch(SectionScaling, func() (err error) {
		overview.Scaling, err = memoized(&c.serviceScaling, s.Cluster+"/"+s.Name, metricsMaxAge, func() (*ECSServiceAutoScaling, error) {
			return c.GetECSServiceAutoScaling(ctx, s.Cluster, s.Name)
		})
		return err
	})

	sections.wait()

	return overview
}
