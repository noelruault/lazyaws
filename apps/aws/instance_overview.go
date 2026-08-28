package aws

import (
	"context"
	"sync"
	"time"
)

// The InstanceOverview.Errs keys, one per fetch rather than one per rendered section: several sections read the same response, so a failed DescribeInstances has to be reportable once and rendered against each section that needed it.
const (
	SectionDetails   = "details"
	SectionStatus    = "status"
	SectionMetrics   = "metrics"
	SectionASG       = "asg"
	SectionAlarms    = "alarms"
	SectionEIP       = "eip"
	SectionConsole   = "console"
	SectionSnapshots = "snapshots"
)

// ConsoleAvailability is what the overview says about the console diagnostics without keeping them: sizes and the capture time, because AWS writes the console log at boot and never again, so the age is the only thing that says whether the content is worth opening.
type ConsoleAvailability struct {
	OutputBytes     int
	CapturedAt      time.Time
	ScreenshotBytes int
}

// InstanceOverview aggregates what the Config, Status, Metrics, Storage, Security and Tags tabs each fetch separately today.
// Every field is independently optional: a throttled or denied call lands in Errs and costs its own section, not the pane.
type InstanceOverview struct {
	Details *InstanceDetails
	Status  *InstanceStatus
	Metrics *InstanceMetrics

	// ASG, Alarms and the Details.ElasticIPs list are filled by the caller, not by GetInstanceOverview, because their refresh frequency is a cost decision rather than a display one.
	// DescribeAlarms cannot filter by dimension server-side, so it pages every alarm in the account against the tightest quota this app touches; all three belong to a selection, never to a ticker.
	ASG    *ASGMembership
	Alarms []InstanceAlarm

	// Console and Snapshots ride the same selection-time schedule: the console output is downloaded whole just to report its size, and the snapshot list is one more describe per volume set — neither belongs on a ticker.
	Console   *ConsoleAvailability
	Snapshots []VolumeSnapshot

	// ExtrasPending marks a render made before the selection-time sections arrived, so the formatter can say "…" where an empty field would otherwise claim an absence nothing has verified yet.
	ExtrasPending bool

	Errs map[string]error
}

// Err reports the fetch error for a section, if that section failed.
func (o *InstanceOverview) Err(section string) error {
	return o.Errs[section]
}

// GetInstanceOverview fetches the three refreshable sections concurrently and always returns an overview, never an error.
// A section that failed is reported through Errs so the sections that succeeded still render: the point of the fan-out is that one denied permission degrades one block instead of blanking the pane.
// metricsMaxAge puts the metrics section on its own slower tier: the pane redraws on a couple of seconds, and re-paying a per-metric bill at that rate for numbers CloudWatch publishes once a minute is what the tier exists to stop. 0 means the reading taken for this selection is reused for as long as it is selected.
func (c *Client) GetInstanceOverview(ctx context.Context, instanceID string, metricsMaxAge time.Duration) *InstanceOverview {
	overview := &InstanceOverview{Errs: map[string]error{}}
	sections := newSectionFetcher(overview.Errs)

	// describeInstance rather than GetInstanceDetails: the Elastic IP list it would add costs its own DescribeAddresses call, and this runs on a refresh ticker.
	sections.fetch(SectionDetails, func() (err error) {
		overview.Details, err = c.describeInstance(ctx, instanceID)
		return err
	})
	sections.fetch(SectionStatus, func() (err error) {
		overview.Status, err = c.GetInstanceStatus(ctx, instanceID)
		return err
	})
	sections.fetch(SectionMetrics, func() (err error) {
		overview.Metrics, err = c.GetInstanceMetricsAged(ctx, instanceID, metricsMaxAge)
		return err
	})

	sections.wait()

	return overview
}

// sectionFetcher runs each of an overview's sections concurrently and collects the failures by section.
// Each fetch writes its own field on the overview, so only the shared error map needs the lock; wait is what publishes every field to the caller.
type sectionFetcher struct {
	mu   sync.Mutex
	wg   sync.WaitGroup
	errs map[string]error
}

func newSectionFetcher(errs map[string]error) *sectionFetcher {
	return &sectionFetcher{errs: errs}
}

func (f *sectionFetcher) fetch(section string, run func() error) {
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		if err := run(); err != nil {
			f.mu.Lock()
			f.errs[section] = err
			f.mu.Unlock()
		}
	}()
}

func (f *sectionFetcher) wait() { f.wg.Wait() }
