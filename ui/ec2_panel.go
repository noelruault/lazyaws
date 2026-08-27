package ui

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"sync"
	"time"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/config"
	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/presentation"
	"github.com/noelruault/lazyaws/ui/tasks"
	"github.com/noelruault/lazyaws/ui/utils"
)

func (gui *Gui) getEC2Panel() *panels.SideListPanel[*aws.Instance] {
	return &panels.SideListPanel[*aws.Instance]{
		ContextState: &panels.ContextState[*aws.Instance]{
			GetMainTabs: func() []panels.MainTab[*aws.Instance] {
				return []panels.MainTab[*aws.Instance]{
					overviewTab(gui, gui.instanceOverview),
					{Key: "config", Title: "Config", Render: gui.renderEC2Config},
					{Key: "status", Title: "Status", Render: gui.renderEC2Status},
					{Key: "metrics", Title: "Metrics", Render: gui.renderEC2Metrics},
					{Key: "storage", Title: "Storage", Render: gui.renderEC2Storage},
					{Key: "security", Title: "Security", Render: gui.renderEC2Security},
					{Key: "tags", Title: "Tags", Render: gui.renderEC2Tags},
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
		Weights: func(*aws.Instance) []int { return presentation.InstanceWeights() },
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
	errs       map[string]error
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
	}

	overview.ASG, overview.Alarms = e.asg, e.alarms
	// The Elastic IPs belong to the details the formatter reads, but they are fetched on this schedule rather than with the rest of them.
	if overview.Details != nil {
		overview.Details.ElasticIPs = e.eips
	}
	maps.Copy(overview.Errs, e.errs)
}

func (gui *Gui) renderEC2Config(inst *aws.Instance) tasks.TaskFunc {
	id := inst.ID
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		details, err := gui.Client.GetInstanceDetails(fetchCtx, id)
		if gen != gui.Gen {
			return
		}
		if err != nil {
			gui.RenderStringMain("error loading instance details: " + err.Error())
			return
		}

		// ASG lookup is optional, so failures must not hide the instance configuration.
		asg, _ := gui.Client.GetInstanceASGMembership(fetchCtx, id)
		if gen != gui.Gen {
			return
		}
		gui.RenderStringMain(formatEC2Config(details, asg))
	}})
}

func formatEC2Config(d *aws.InstanceDetails, asg *aws.ASGMembership) string {
	out := utils.FormatMap(0, map[string]string{
		"ID":           d.ID,
		"Name":         orNone(d.Name),
		"State":        d.State,
		"Type":         d.InstanceType,
		"AZ":           d.AZ,
		"VPC":          d.VpcID,
		"Subnet":       d.SubnetID,
		"Private IP":   orNone(d.PrivateIP),
		"Public IP":    orNone(d.PublicIP),
		"Key pair":     orNone(d.KeyName),
		"Architecture": d.Architecture,
		"Platform":     orNone(d.Platform),
		"Root device":  d.RootDeviceType,
		"Monitoring":   orNone(d.Monitoring),
		"IAM profile":  orNone(d.IamInstanceProfile),
		"Launch time":  d.LaunchTime,
	})

	if info := d.InstanceTypeInfo; info != nil {
		out += fmt.Sprintf("\nvCPUs: %d, Memory: %.1f GiB, Network: %s, Storage: %s\n",
			info.VCpus, float64(info.Memory)/1024, orNone(info.NetworkPerformance), info.StorageType)
	}

	out += "\nNetwork interfaces:\n"
	if len(d.NetworkInterfaces) == 0 {
		out += "none\n"
	}
	for _, ni := range d.NetworkInterfaces {
		out += fmt.Sprintf("  %s private:%s public:%s subnet:%s\n", ni.ID, orNone(ni.PrivateIP), orNone(ni.PublicIP), ni.SubnetID)
	}

	out += "\nAuto Scaling Group:\n"
	if asg == nil {
		out += "none\n"
	} else {
		out += fmt.Sprintf("  %s (desired %d, min %d, max %d)\n", asg.GroupName, asg.Desired, asg.Min, asg.Max)
	}

	out += "\nElastic IPs:\n"
	if len(d.ElasticIPs) == 0 {
		out += "none\n"
	}
	for _, eip := range d.ElasticIPs {
		out += fmt.Sprintf("  %s (assoc:%s, ni:%s)\n", eip.PublicIP, orNone(eip.AssociationID), orNone(eip.NetworkIF))
	}

	return out
}

func (gui *Gui) renderEC2Status(inst *aws.Instance) tasks.TaskFunc {
	id := inst.ID
	return gui.NewTickerTask(TickerTaskOpts{
		Duration: config.RefreshInterval(gui.Config.User.Refresh.EC2StatusSeconds, 10),
		Before:   func(ctx context.Context) { gui.clearMainView() },
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			gen := gui.Gen
			fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			status, err := gui.Client.GetInstanceStatus(fetchCtx, id)
			if gen != gui.Gen {
				return
			}
			if err != nil {
				gui.reRenderStringMain("error loading instance status: " + err.Error())
				return
			}

			// Optional diagnostics must not hide status or scheduled events when one lookup fails.
			alarms, _ := gui.Client.GetInstanceAlarms(fetchCtx, id)
			consoleOutput, _ := gui.Client.GetConsoleOutput(fetchCtx, id)
			consoleScreenshot, _ := gui.Client.GetConsoleScreenshot(fetchCtx, id)
			if gen != gui.Gen {
				return
			}
			gui.reRenderStringMain(formatEC2Status(status, alarms, consoleOutput, consoleScreenshot, time.Now()))
		},
	})
}

func formatEC2Status(s *aws.InstanceStatus, alarms []aws.InstanceAlarm, consoleOutput aws.ConsoleOutput, consoleScreenshot string, now time.Time) string {
	out := utils.FormatMap(0, map[string]string{
		"Instance state":  s.InstanceState,
		"System status":   orNone(s.SystemStatus),
		"Instance status": orNone(s.InstanceStatus),
	})

	out += "\nScheduled events:\n"
	if len(s.ScheduledEvents) == 0 {
		out += "none\n"
	}
	for _, e := range s.ScheduledEvents {
		out += fmt.Sprintf("  %s %s (%s - %s)\n", e.Code, e.Description, e.NotBefore, e.NotAfter)
	}

	out += "\nCloudWatch alarms:\n"
	if len(alarms) == 0 {
		out += "none\n"
	}
	for _, a := range alarms {
		out += fmt.Sprintf("  %s %s (%s)\n", presentation.StatusCell(a.State, presentation.StatusStyleIcon), a.Name, a.MetricName)
	}

	out += "\nConsole output:\n"
	if consoleOutput.Content == "" {
		out += "none\n"
	} else {
		out += fmt.Sprintf("available (%s), captured %s\n", formatByteCount(float64(len(consoleOutput.Content)*3/4)), consoleCapture(consoleOutput.At, now))
	}

	out += "\nConsole screenshot:\n"
	if consoleScreenshot == "" {
		out += "none\n"
	} else {
		out += fmt.Sprintf("available (%s)\n", formatByteCount(float64(len(consoleScreenshot)*3/4)))
	}

	return out
}

// consoleCapture dates the console log against now, because AWS captures it at boot and never again: on an instance up for months the size looks like a live log and the age is the only thing that says otherwise.
func consoleCapture(at, now time.Time) string {
	if at.IsZero() {
		return "unknown"
	}

	return at.UTC().Format(time.RFC3339) + " (" + presentation.RelTime(at, now) + ")"
}

func (gui *Gui) renderEC2Metrics(inst *aws.Instance) tasks.TaskFunc {
	id := inst.ID
	return gui.NewTickerTask(TickerTaskOpts{
		Duration: config.RefreshInterval(gui.Config.User.Refresh.EC2StatusSeconds, 10),
		Before:   func(ctx context.Context) { gui.clearMainView() },
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			gen := gui.Gen
			fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			metrics, err := gui.Client.GetInstanceMetrics(fetchCtx, id)
			if gen != gui.Gen {
				return
			}
			if err != nil {
				gui.reRenderStringMain("error loading metrics: " + err.Error())
				return
			}
			gui.reRenderStringMain(formatEC2Metrics(metrics))
		},
	})
}

// formatMetricPoint keeps the panel call sites short now that the formatter is shared with the overview.
func formatMetricPoint(p aws.MetricPoint, stat string, format func(float64) string) string {
	return presentation.MetricReading(p, stat, format)
}

func formatEC2Metrics(m *aws.InstanceMetrics) string {
	percent := func(v float64) string { return fmt.Sprintf("%.1f%%", v) }
	count := func(v float64) string { return strconv.Itoa(int(v)) }
	return utils.FormatMap(0, map[string]string{
		"CPU utilization":     formatMetricPoint(m.CPUUtilization, "5-min avg", percent),
		"Network in":          formatMetricPoint(m.NetworkIn, "5-min total", formatByteCount),
		"Network out":         formatMetricPoint(m.NetworkOut, "5-min total", formatByteCount),
		"Disk read":           formatMetricPoint(m.DiskReadBytes, "5-min total", formatByteCount),
		"Disk write":          formatMetricPoint(m.DiskWriteBytes, "5-min total", formatByteCount),
		"Status check failed": formatMetricPoint(m.StatusCheckFailed, "5-min max", count),
	})
}

// formatByteCount keeps the panel call sites short now that the formatter is shared with the presentation overviews.
func formatByteCount(b float64) string {
	return presentation.FormatByteCount(b)
}

// renderEC2Storage refetches per tab and treats optional snapshots as best effort.
func (gui *Gui) renderEC2Storage(inst *aws.Instance) tasks.TaskFunc {
	id := inst.ID
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		details, err := gui.Client.GetInstanceDetails(fetchCtx, id)
		if gen != gui.Gen {
			return
		}
		if err != nil {
			gui.RenderStringMain("error loading storage: " + err.Error())
			return
		}

		snapshots, err := gui.Client.ListVolumeSnapshots(fetchCtx, ec2VolumeIDs(details.BlockDevices))
		if gen != gui.Gen {
			return
		}
		if err != nil {
			gui.Log.Warn(err.Error())
		}
		gui.RenderStringMain(formatEC2Storage(details.BlockDevices, snapshots))
	}})
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

func formatEC2Storage(devices []aws.BlockDevice, snapshots []aws.VolumeSnapshot) string {
	if len(devices) == 0 {
		return "no EBS volumes\n"
	}
	rows := make([][]string, len(devices))
	for i, d := range devices {
		enc := "not encrypted"
		if d.Encrypted {
			enc = "encrypted"
		}
		rows[i] = []string{
			d.DeviceName,
			d.VolumeID,
			fmt.Sprintf("%d GiB", d.VolumeSize),
			d.VolumeType,
			fmt.Sprintf("%d IOPS", d.Iops),
			fmt.Sprintf("%d MiB/s", d.Throughput),
			enc,
		}
	}
	table, err := utils.RenderTable(rows)
	if err != nil {
		return err.Error()
	}

	out := table + "\nSnapshots:\n"
	if len(snapshots) == 0 {
		out += "none\n"
	}
	for _, s := range snapshots {
		out += fmt.Sprintf("  %s vol:%s %s %s (%d GiB) started %s\n", s.SnapshotID, s.VolumeID, s.State, s.Progress, s.SizeGiB, s.StartTime)
	}
	return out
}

func (gui *Gui) renderEC2Security(inst *aws.Instance) tasks.TaskFunc {
	id := inst.ID
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		details, err := gui.Client.GetInstanceDetails(fetchCtx, id)
		if gen != gui.Gen {
			return
		}
		if err != nil {
			gui.RenderStringMain("error loading security groups: " + err.Error())
			return
		}
		gui.RenderStringMain(formatEC2Security(details.SecurityGroups))
	}})
}

func formatEC2Security(groups []aws.SecurityGroup) string {
	if len(groups) == 0 {
		return "no security groups\n"
	}
	out := "Security groups:\n"
	for _, sg := range groups {
		out += fmt.Sprintf("  %s (%s)\n", sg.Name, sg.ID)
	}
	return out
}

// renderEC2Tags reuses ListInstances data instead of making another AWS call.
func (gui *Gui) renderEC2Tags(inst *aws.Instance) tasks.TaskFunc {
	tagList := inst.Tags
	return gui.NewSimpleRenderStringTask(func() string {
		m := make(map[string]string, len(tagList))
		for _, t := range tagList {
			m[t.Key] = t.Value
		}
		return utils.FormatMap(0, m)
	})
}
