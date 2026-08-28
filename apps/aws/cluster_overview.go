package aws

import (
	"context"
	"time"
)

// The ECSClusterOverview.Errs keys. SectionMetrics is shared with the instance overview: the key names the fetch that failed, and both panes have exactly one metrics fetch.
const (
	SectionServices = "services"
	SectionTasks    = "tasks"
)

// ECSClusterOverview aggregates what the cluster's Config, Instances and Tags tabs each fetch separately today.
// Every field is independently optional: a throttled or denied call lands in Errs and costs its own section, not the pane.
type ECSClusterOverview struct {
	Services []ECSService
	Tasks    []ECSTask
	Metrics  *ECSClusterMetrics
	Tags     map[string]string

	// InsightsOff records that the metrics call was deliberately NOT made, which is a different answer from a call that failed and a different one again from a call that came back empty.
	// Without it a cluster with Insights switched off renders the same "no data" as a cluster whose metrics are genuinely missing, and only one of those is something to go and fix.
	InsightsOff bool

	Errs map[string]error
}

// Err reports the fetch error for a section, if that section failed.
func (o *ECSClusterOverview) Err(section string) error {
	return o.Errs[section]
}

// GetECSClusterOverview fetches the cluster's sections concurrently and always returns an overview, never an error.
// A section that failed is reported through Errs so the sections that succeeded still render: one denied permission degrades one block instead of blanking the pane.
// metricsMaxAge puts the metrics section on its own slower tier, for the reason GetInstanceOverview documents; 0 means the reading taken for this selection is reused for as long as it is selected.
func (c *Client) GetECSClusterOverview(ctx context.Context, cluster *ECSCluster, metricsMaxAge time.Duration) *ECSClusterOverview {
	overview := &ECSClusterOverview{Errs: map[string]error{}}
	sections := newSectionFetcher(overview.Errs)

	sections.fetch(SectionServices, func() (err error) {
		overview.Services, err = c.ListECSServices(ctx, cluster.Name)
		return err
	})
	// The empty service name is the whole cluster: one ListTasks plus one DescribeTasks page, rather than the per-service call that would make this an N+1 on a refresh ticker.
	sections.fetch(SectionTasks, func() (err error) {
		overview.Tasks, err = c.ListECSTasks(ctx, cluster.Name, "")
		return err
	})
	sections.fetch(SectionTags, func() (err error) {
		overview.Tags, err = c.listClusterTags(ctx, cluster.Name)
		return err
	})

	// The setting is read off the cluster in hand rather than the recorded one, because this pane always has the described cluster and the record exists for callers that do not.
	if !ContainerInsightsEnabled(cluster.ContainerInsights) {
		overview.InsightsOff = true

		sections.wait()

		return overview
	}

	sections.fetch(SectionMetrics, func() (err error) {
		overview.Metrics, err = memoized(&c.clusterMetrics, cluster.Name, metricsMaxAge, func() (*ECSClusterMetrics, error) {
			return c.GetECSClusterMetrics(ctx, cluster.Name)
		})
		return err
	})

	sections.wait()

	return overview
}
