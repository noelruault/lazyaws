package aws

import (
	"context"
	"sync"
)

// The InstanceOverview.Errs keys, one per fetch rather than one per rendered section: several sections read the same response, so a failed DescribeInstances has to be reportable once and rendered against each section that needed it.
const (
	SectionDetails = "details"
	SectionStatus  = "status"
	SectionMetrics = "metrics"
	SectionASG     = "asg"
	SectionAlarms  = "alarms"
)

// InstanceOverview aggregates what the Config, Status, Metrics, Storage, Security and Tags tabs each fetch separately today.
// Every field is independently optional: a throttled or denied call lands in Errs and costs its own section, not the pane.
type InstanceOverview struct {
	Details *InstanceDetails
	Status  *InstanceStatus
	Metrics *InstanceMetrics

	// ASG and Alarms are filled by the caller, not by GetInstanceOverview, because they are the two sections whose refresh frequency is a cost decision rather than a display one.
	// DescribeAlarms cannot filter by dimension server-side, so it pages every alarm in the account against the tightest quota this app touches; it belongs to a selection, never to a ticker.
	ASG    *ASGMembership
	Alarms []InstanceAlarm

	Errs map[string]error
}

// Err reports the fetch error for a section, if that section failed.
func (o *InstanceOverview) Err(section string) error {
	return o.Errs[section]
}

// GetInstanceOverview fetches the three refreshable sections concurrently and always returns an overview, never an error.
// A section that failed is reported through Errs so the sections that succeeded still render: the point of the fan-out is that one denied permission degrades one block instead of blanking the pane.
func (c *Client) GetInstanceOverview(ctx context.Context, instanceID string) *InstanceOverview {
	overview := &InstanceOverview{Errs: map[string]error{}}

	var mu sync.Mutex
	var wg sync.WaitGroup

	// Each fetch writes its own field, so only the shared error map needs the lock; wg.Wait is what publishes the fields to the caller.
	fetch := func(section string, run func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := run(); err != nil {
				mu.Lock()
				overview.Errs[section] = err
				mu.Unlock()
			}
		}()
	}

	fetch(SectionDetails, func() (err error) {
		overview.Details, err = c.GetInstanceDetails(ctx, instanceID)
		return err
	})
	fetch(SectionStatus, func() (err error) {
		overview.Status, err = c.GetInstanceStatus(ctx, instanceID)
		return err
	})
	fetch(SectionMetrics, func() (err error) {
		overview.Metrics, err = c.GetInstanceMetrics(ctx, instanceID)
		return err
	})

	wg.Wait()

	return overview
}
