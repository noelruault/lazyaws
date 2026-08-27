package ui

import (
	"context"
	"maps"
	"sync"
	"time"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/presentation"
	"github.com/noelruault/lazyaws/ui/utils"
)

func (gui *Gui) getEC2Panel() *panels.SideListPanel[*aws.Instance] {
	return &panels.SideListPanel[*aws.Instance]{
		ContextState: &panels.ContextState[*aws.Instance]{
			// One tab: with the console diagnostics and volume snapshots folded in, the Overview carries everything the six detail tabs held, and a tab that only repeats the pane beside it is navigation debt.
			GetMainTabs: func() []panels.MainTab[*aws.Instance] {
				return []panels.MainTab[*aws.Instance]{
					overviewTab(gui, gui.instanceOverview),
				}
			},
			GetItemContextCacheKey: func(inst *aws.Instance) string {
				return "ec2-" + inst.ID
			},
		},

		ListPanel: panels.ListPanel[*aws.Instance]{
			List: panels.NewFilteredList[*aws.Instance](),
			View: gui.Views.EC2,
		},
		NoItemsMessage: "no EC2 instances",
		Gui:            gui.intoInterface(),

		Sort: func(a, b *aws.Instance) bool {
			aRunning := a.State == "running"
			bRunning := b.State == "running"
			if aRunning != bRunning {
				return aRunning
			}
			return a.Name < b.Name
		},
		GetTableCellsFit: func(inst *aws.Instance) []utils.Cell {
			return presentation.GetInstanceDisplayCells(inst)
		},
		Weights:   func(*aws.Instance) []int { return presentation.InstanceWeights() },
		CopyValue: func(inst *aws.Instance) string { return inst.ID },
	}
}

func (gui *Gui) loadEC2List() error {
	if gui.Client == nil {
		return nil
	}

	gen := gui.Gen

	return gui.WithWaitingStatus("loading ec2", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		instances, err := gui.Client.ListInstances(ctx)
		if err != nil {
			return err
		}
		if gen != gui.Gen {
			return nil
		}

		rows := make([]*aws.Instance, len(instances))
		for i := range instances {
			rows[i] = &instances[i]
		}
		gui.Panels.EC2.SetItemsKeepSelection(rows, ec2SelectionKey)
		return gui.Panels.EC2.RerenderList()
	})
}

// ec2SelectionKey identifies an instance across reloads. The Name tag is absent on plenty of instances and duplicated across an autoscaling group, so the id is the only identity.
func ec2SelectionKey(inst *aws.Instance) string { return inst.ID }

// instanceOverview consolidates the six detail tabs into one pane, refetching the refreshable sections on every render and reusing the selection-time ones.
func (gui *Gui) instanceOverview(ctx context.Context, inst *aws.Instance, width int) string {
	if gui.Client == nil {
		return overviewUnavailable("instance")
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	overview := gui.Client.GetInstanceOverview(fetchCtx, inst.ID, gui.metricsMaxAge())
	gui.ec2Extras.fill(fetchCtx, gui.Client, gui.Gen, inst.ID, overview)
	gui.throttles.observeSections(overview.Errs)

	return presentation.FormatInstanceOverview(inst, overview, width, time.Now())
}

// ec2OverviewExtras holds the two overview sections whose frequency is a cost decision rather than a display one.
// The overview re-renders on a ticker, and DescribeAlarms cannot filter by dimension server-side: it pages every alarm in the account, against the tightest quota this app touches. Fetching that every couple of seconds is what "best effort" must not turn into, so both are fetched once per selected instance and reused until the selection moves.
type ec2OverviewExtras struct {
	mu         sync.Mutex
	gen        int
	instanceID string
	asg        *aws.ASGMembership
	alarms     []aws.InstanceAlarm
	eips       []aws.ElasticIP
	console    *aws.ConsoleAvailability
	errs       map[string]error

	// Snapshots latch on their own flag because they need the volume ids off the ticker's details fetch: on a render where details failed, the other extras still fill and this one waits for a render that has them.
	snapsFilled bool
	snapshots   []aws.VolumeSnapshot
	snapsErr    error
}

// fill puts the selection-time sections onto an overview, fetching them only when the selection or the profile behind it has moved.
// The lock is held across the fetches on purpose: it is also what stops two overview renders of the same instance from making the same pair of calls at once.
func (e *ec2OverviewExtras) fill(ctx context.Context, client *aws.Client, gen int, instanceID string, overview *aws.InstanceOverview) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// The generation is part of the key because an instance id is only unique within the account it was read from, and a profile switch replaces the account without changing the id.
	if e.instanceID != instanceID || e.gen != gen {
		e.instanceID, e.gen = instanceID, gen
		e.errs = map[string]error{}
		e.snapsFilled, e.snapshots, e.snapsErr = false, nil, nil

		asg, err := client.GetInstanceASGMembership(ctx, instanceID)
		e.asg = asg
		if err != nil {
			e.errs[aws.SectionASG] = err
		}

		alarms, err := client.GetInstanceAlarms(ctx, instanceID)
		e.alarms = alarms
		if err != nil {
			e.errs[aws.SectionAlarms] = err
		}

		eips, err := client.DescribeInstanceAddresses(ctx, instanceID)
		e.eips = eips
		if err != nil {
			e.errs[aws.SectionEIP] = err
		}

		e.console = fetchConsoleAvailability(ctx, client, instanceID, e.errs)
	}

	if !e.snapsFilled && overview.Details != nil {
		e.snapsFilled = true
		e.snapshots, e.snapsErr = client.ListVolumeSnapshots(ctx, ec2VolumeIDs(overview.Details.BlockDevices))
	}

	overview.ASG, overview.Alarms, overview.Console = e.asg, e.alarms, e.console
	overview.Snapshots = e.snapshots
	if e.snapsErr != nil {
		overview.Errs[aws.SectionSnapshots] = e.snapsErr
	}
	// The Elastic IPs belong to the details the formatter reads, but they are fetched on this schedule rather than with the rest of them.
	if overview.Details != nil {
		overview.Details.ElasticIPs = e.eips
	}
	maps.Copy(overview.Errs, e.errs)
}

// fetchConsoleAvailability reduces the two console diagnostics to their sizes and capture time on the spot, so the payloads this exists to avoid re-downloading are not kept in memory either.
func fetchConsoleAvailability(ctx context.Context, client *aws.Client, instanceID string, errs map[string]error) *aws.ConsoleAvailability {
	output, outErr := client.GetConsoleOutput(ctx, instanceID)
	screenshot, shotErr := client.GetConsoleScreenshot(ctx, instanceID)
	if outErr != nil && shotErr != nil {
		errs[aws.SectionConsole] = outErr
		return nil
	}

	// Both are base64 on the wire, so the real size is 3/4 of what arrived.
	return &aws.ConsoleAvailability{
		OutputBytes:     len(output.Content) * 3 / 4,
		CapturedAt:      output.At,
		ScreenshotBytes: len(screenshot) * 3 / 4,
	}
}

// formatByteCount keeps the panel call sites short now that the formatter is shared with the presentation overviews.
func formatByteCount(b float64) string {
	return presentation.FormatByteCount(b)
}

func ec2VolumeIDs(devices []aws.BlockDevice) []string {
	ids := make([]string, 0, len(devices))
	for _, d := range devices {
		if d.VolumeID != "" {
			ids = append(ids, d.VolumeID)
		}
	}
	return ids
}
