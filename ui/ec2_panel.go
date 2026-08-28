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
					overviewTab(gui, func(inst *aws.Instance) string { return "ec2-" + inst.ID }, gui.instanceOverview),
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
// When the extras still have to be fetched it paints the pane once WITHOUT them first: against real AWS they are the slow half of the render, and everything above them is already in hand.
func (gui *Gui) instanceOverview(ctx context.Context, inst *aws.Instance, width int) string {
	if gui.Client == nil {
		return overviewUnavailable("instance")
	}

	gen := gui.Gen
	fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	overview := gui.Client.GetInstanceOverview(fetchCtx, inst.ID, gui.metricsMaxAge())
	if !gui.ec2Extras.has(gen, inst.ID) && gen == gui.Gen {
		overview.ExtrasPending = true
		// Ordered like renderOverview's own writes, or this early paint could land after the full one and resurrect the pending ellipses.
		gui.reRenderStringMainOrdered(presentation.FormatInstanceOverview(inst, overview, width, time.Now()))
		overview.ExtrasPending = false
	}
	gui.ec2Extras.fill(fetchCtx, gui.Client, gen, inst.ID, overview)
	gui.throttles.observeSections(overview.Errs)

	return presentation.FormatInstanceOverview(inst, overview, width, time.Now())
}

// ec2OverviewExtras holds the overview sections whose frequency is a cost decision rather than a display one, one entry per instance visited.
// The overview re-renders on a ticker, and DescribeAlarms cannot filter by dimension server-side: it pages every alarm in the account, against the tightest quota this app touches. Fetching that every couple of seconds is what "best effort" must not turn into, so each instance is fetched once and kept, and moving back and forth between instances repays nothing.
// Unbounded like the metrics memos: one small struct per instance visited this session, dropped whole on a profile switch.
type ec2OverviewExtras struct {
	mu      sync.Mutex
	gen     int
	entries map[string]*ec2ExtrasEntry
}

type ec2ExtrasEntry struct {
	asg     *aws.ASGMembership
	alarms  []aws.InstanceAlarm
	eips    []aws.ElasticIP
	console *aws.ConsoleAvailability
	errs    map[string]error

	// Snapshots latch on their own flag because they need the volume ids off the ticker's details fetch: on a render where details failed, the other extras still fill and this one waits for a render that has them.
	snapsFilled bool
	snapshots   []aws.VolumeSnapshot
	snapsErr    error
}

// has reports whether the instance's extras are already fetched, which is what decides if a render paints a pending pane first.
func (e *ec2OverviewExtras) has(gen int, instanceID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.entries[instanceID]
	return e.gen == gen && ok
}

// fill puts the selection-time sections onto an overview, fetching them only when the instance has not been visited under this profile.
// The lock is held across the fetches on purpose: it is also what stops two overview renders of the same instance from making the same calls at once.
// ponytail: one lock across all instances, so a render of instance B waits out instance A's in-flight fetch; per-entry locks if panel hopping ever feels it.
func (e *ec2OverviewExtras) fill(ctx context.Context, client *aws.Client, gen int, instanceID string, overview *aws.InstanceOverview) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// The generation is part of the key because an instance id is only unique within the account it was read from, and a profile switch replaces the account without changing the id.
	if e.gen != gen || e.entries == nil {
		e.gen, e.entries = gen, map[string]*ec2ExtrasEntry{}
	}

	entry, ok := e.entries[instanceID]
	if !ok {
		entry = &ec2ExtrasEntry{errs: map[string]error{}}
		e.entries[instanceID] = entry

		// Concurrently: these are independent reads of independent services, and serially their latencies add up to the pane's whole first paint.
		var wg sync.WaitGroup
		var errsMu sync.Mutex
		fail := func(section string, err error) {
			if err == nil {
				return
			}
			errsMu.Lock()
			entry.errs[section] = err
			errsMu.Unlock()
		}
		wg.Add(4)
		go func() {
			defer wg.Done()
			asg, err := client.GetInstanceASGMembership(ctx, instanceID)
			entry.asg = asg
			fail(aws.SectionASG, err)
		}()
		go func() {
			defer wg.Done()
			alarms, err := client.GetInstanceAlarms(ctx, instanceID)
			entry.alarms = alarms
			fail(aws.SectionAlarms, err)
		}()
		go func() {
			defer wg.Done()
			eips, err := client.DescribeInstanceAddresses(ctx, instanceID)
			entry.eips = eips
			fail(aws.SectionEIP, err)
		}()
		go func() {
			defer wg.Done()
			console, err := fetchConsoleAvailability(ctx, client, instanceID)
			entry.console = console
			fail(aws.SectionConsole, err)
		}()
		wg.Wait()
	}

	if !entry.snapsFilled && overview.Details != nil {
		entry.snapsFilled = true
		entry.snapshots, entry.snapsErr = client.ListVolumeSnapshots(ctx, ec2VolumeIDs(overview.Details.BlockDevices))
	}

	overview.ASG, overview.Alarms, overview.Console = entry.asg, entry.alarms, entry.console
	overview.Snapshots = entry.snapshots
	if entry.snapsErr != nil {
		overview.Errs[aws.SectionSnapshots] = entry.snapsErr
	}
	// The Elastic IPs belong to the details the formatter reads, but they are fetched on this schedule rather than with the rest of them.
	if overview.Details != nil {
		overview.Details.ElasticIPs = entry.eips
	}
	maps.Copy(overview.Errs, entry.errs)
}

// fetchConsoleAvailability reduces the two console diagnostics to their sizes and capture time on the spot, so the payloads this exists to avoid re-downloading are not kept in memory either.
func fetchConsoleAvailability(ctx context.Context, client *aws.Client, instanceID string) (*aws.ConsoleAvailability, error) {
	output, outErr := client.GetConsoleOutput(ctx, instanceID)
	screenshot, shotErr := client.GetConsoleScreenshot(ctx, instanceID)
	if outErr != nil && shotErr != nil {
		return nil, outErr
	}

	// Both are base64 on the wire, so the real size is 3/4 of what arrived.
	return &aws.ConsoleAvailability{
		OutputBytes:     len(output.Content) * 3 / 4,
		CapturedAt:      output.At,
		ScreenshotBytes: len(screenshot) * 3 / 4,
	}, nil
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
