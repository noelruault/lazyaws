package ui

import (
	"context"
	"fmt"
	"strconv"
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
			gui.reRenderStringMain(formatEC2Status(status, alarms, consoleOutput, consoleScreenshot))
		},
	})
}

func formatEC2Status(s *aws.InstanceStatus, alarms []aws.InstanceAlarm, consoleOutput, consoleScreenshot string) string {
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
	if consoleOutput == "" {
		out += "none\n"
	} else {
		out += fmt.Sprintf("available (%s)\n", formatByteCount(float64(len(consoleOutput)*3/4)))
	}

	out += "\nConsole screenshot:\n"
	if consoleScreenshot == "" {
		out += "none\n"
	} else {
		out += fmt.Sprintf("available (%s)\n", formatByteCount(float64(len(consoleScreenshot)*3/4)))
	}

	return out
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

func formatEC2Metrics(m *aws.InstanceMetrics) string {
	return utils.FormatMap(0, map[string]string{
		"Period":              m.Period,
		"CPU utilization":     fmt.Sprintf("%.1f%%", m.CPUUtilization),
		"Network in":          formatByteCount(m.NetworkIn),
		"Network out":         formatByteCount(m.NetworkOut),
		"Disk read":           formatByteCount(m.DiskReadBytes),
		"Disk write":          formatByteCount(m.DiskWriteBytes),
		"Status check failed": strconv.Itoa(m.StatusCheckFailed),
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
