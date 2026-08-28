package presentation

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// InstanceWeights shares the slack between the name and the instance id, twice as much to the name.
// The id has to flex rather than size to its content: it is 19 cells, and with the icon, type and private IP that is the whole of a 40-cell side panel, which leaves a name column of zero width.
// A row that drops the name entirely to keep four narrower columns is the wrong trade — squeezing both text columns and cutting each with an ellipsis keeps every field on screen.
func InstanceWeights() []int {
	return []int{0, 2, 1, 0, 0}
}

func GetInstanceDisplayCells(inst *aws.Instance) []utils.Cell {
	name := inst.Name
	if name == "" {
		name = "(no name)"
	}

	return []utils.Cell{
		StatusCellFit(inst.State, StatusStyleIcon),
		{Text: name, Color: color.Bold},
		// The instance id is muted because it is the fallback identifier: you read it when the name is missing or ambiguous, not on every glance down the list.
		{Text: inst.ID, Color: color.Faint},
		{Text: inst.InstanceType},
		{Text: inst.PrivateIP},
	}
}

// MetricReading stamps a CloudWatch reading with the time CloudWatch published it, because the freshest datapoint basic monitoring offers is already minutes old and captioning it "last 5 minutes" claims a freshness the data does not have.
// A series that published nothing reads "no data": zero is a measurement, absence is not.
func MetricReading(p aws.MetricPoint, stat string, format func(float64) string) string {
	if !p.OK {
		return "no data"
	}

	return fmt.Sprintf("%s (%s @ %s)", format(p.Value), stat, p.At.UTC().Format("15:04Z"))
}

// FormatInstanceOverview lays an instance out for the Overview tab: a header that always renders, then the two-column body the six detail tabs are consolidated into.
// The header is built from the list row rather than from the fetch, so an instance whose every section failed is still identified by name and id instead of leaving an anonymous pane of errors.
func FormatInstanceOverview(inst *aws.Instance, o *aws.InstanceOverview, width int, now time.Time) string {
	// Cut to the pane: the header spans the full width rather than a column, so Columns never sees it, and with wrap off a long name plus its badge and id runs off the edge unmarked.
	header := truncateBlock(ResourceHeader("EC2 Instance", instanceName(inst), Badge(inst.State), inst.ID, inst.InstanceType, inst.AZ), width)

	column := ColumnWidth(width, overviewGap)
	left := joinBlocks(
		instanceConfigBlock(o, now),
		instanceNetworkBlock(o),
		instanceMetricsBlock(o),
	)
	right := joinBlocks(
		instanceStatusBlock(o),
		instanceStorageBlock(o, column),
		instanceSecurityBlock(o),
		instanceConsoleBlock(o, now),
		instanceTagsBlock(o, column),
	)

	return header + "\n\n" + Columns(width, overviewGap, left, right)
}

// overviewGap is the blank cells Columns leaves on each side of its rule, fixed here so the width the storage table is built for is the width its column is cut to.
const overviewGap = 2

// joinBlocks separates sections with a blank line, which is the only thing keeping two headings from reading as one list once a section is a single line long.
func joinBlocks(blocks ...string) string {
	return strings.Join(blocks, "\n\n")
}

func instanceName(inst *aws.Instance) string {
	if inst.Name == "" {
		return "(no name)"
	}

	return inst.Name
}

// sectionUnavailable states which fetch failed and why, rather than leaving the section blank.
// The reason has to stay on screen because the overview retries on its own interval: without it a transient throttle and a permanent denial look identical, which is the difference between waiting and fixing an IAM policy.
func sectionUnavailable(title string, err error) string {
	return SectionTitle(title) + "\n" + utils.ColoredString("unavailable: "+err.Error(), color.FgRed)
}

// fieldUnavailable is the same statement for ONE row of a section whose other rows read fine, which is where a per-field read failed rather than the fetch behind the whole section.
func fieldUnavailable(err error) string {
	return utils.ColoredString("unavailable: "+err.Error(), color.FgRed)
}

func instanceConfigBlock(o *aws.InstanceOverview, now time.Time) string {
	if err := o.Err(aws.SectionDetails); err != nil {
		return sectionUnavailable("Configuration", err)
	}
	d := o.Details

	rows := []kv{
		{"Type", instanceTypeLine(d)},
		{"AZ", orNone(d.AZ)},
		{"Architecture", orNone(d.Architecture)},
		{"Platform", orNone(d.Platform)},
		{"Key pair", orNone(d.KeyName)},
		{"IAM profile", orNone(d.IamInstanceProfile)},
		{"Root device", orNone(d.RootDeviceType)},
		{"Monitoring", orNone(d.Monitoring)},
		{"Launched", launchedAt(d.LaunchTime, now)},
	}
	// Same rule as instanceTypeLine: absent specs degrade to the row not appearing, never to a value the lookup did not answer.
	if d.InstanceTypeInfo != nil {
		rows = append(rows, kv{"Instance storage", orNone(d.InstanceTypeInfo.StorageType)})
	}

	return SectionTitle("Configuration") + "\n" + kvBlock(rows)
}

// instanceTypeLine folds the cached DescribeInstanceTypes specs onto the type name, so the size means something without a second lookup.
// The specs are absent whenever that call failed, and the type name alone is still the answer to "what is this".
func instanceTypeLine(d *aws.InstanceDetails) string {
	info := d.InstanceTypeInfo
	if info == nil {
		return orNone(d.InstanceType)
	}

	return fmt.Sprintf("%s · %d vCPU · %.1f GiB", orNone(d.InstanceType), info.VCpus, float64(info.Memory)/1024)
}

// launchedAt keeps the launch date and its age together: uptime is what an operator reads it for, and the date is what an audit does.
// LaunchTime is a preformatted string from the list mapping rather than a time.Time, so an unparseable value degrades to the string itself instead of to a wrong age.
func launchedAt(launchTime string, now time.Time) string {
	if launchTime == "" {
		return "none"
	}

	at, err := time.Parse(time.RFC3339, launchTime)
	if err != nil {
		return launchTime
	}

	return launchTime + " (" + RelTime(at, now) + ")"
}

func instanceNetworkBlock(o *aws.InstanceOverview) string {
	if err := o.Err(aws.SectionDetails); err != nil {
		return sectionUnavailable("Network", err)
	}
	d := o.Details

	// Every instance type carries a network performance rating, so the field has no absent state for "none" to describe: a nil InstanceTypeInfo can only be the DescribeInstanceTypes lookup having failed.
	// instanceTypeLine degrades honestly on the same nil because the type name alone still answers what the instance IS; this row has nothing left to fall back to, so it says so.
	performance := utils.ColoredString("unavailable", color.FgRed)
	if d.InstanceTypeInfo != nil {
		performance = orNone(d.InstanceTypeInfo.NetworkPerformance)
	}

	lines := []string{SectionTitle("Network"), kvBlock([]kv{
		{"Private IP", orNone(d.PrivateIP)},
		{"Public IP", orNone(d.PublicIP)},
		{"VPC", orNone(d.VpcID)},
		{"Subnet", orNone(d.SubnetID)},
		{"Performance", performance},
	})}

	lines = append(lines, "Interfaces:")
	if len(d.NetworkInterfaces) == 0 {
		lines = append(lines, "  none")
	}
	for _, ni := range d.NetworkInterfaces {
		lines = append(lines, "  "+ni.ID+" "+utils.ColoredString(interfaceAddresses(ni), color.Faint))
	}

	// An unreadable address list is not an instance with no Elastic IP: the fetch is separate from the rest of the details, so its failure is stated rather than rendered as an absence.
	if err := o.Err(aws.SectionEIP); err != nil {
		return strings.Join(append(lines, "Elastic IPs: "+utils.ColoredString("unavailable", color.FgRed)), "\n")
	}
	if o.ExtrasPending {
		return strings.Join(append(lines, "Elastic IPs: "+pendingValue()), "\n")
	}

	lines = append(lines, "Elastic IPs:")
	if len(d.ElasticIPs) == 0 {
		lines = append(lines, "  none")
	}
	for _, eip := range d.ElasticIPs {
		line := "  " + eip.PublicIP
		if eip.NetworkIF != "" {
			line += " " + utils.ColoredString(eip.NetworkIF, color.Faint)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// interfaceAddresses compacts one interface's addressing to its row: private always, public and subnet when present.
func interfaceAddresses(ni aws.NetworkInterface) string {
	parts := []string{orNone(ni.PrivateIP)}
	if ni.PublicIP != "" {
		parts = append(parts, "→ "+ni.PublicIP)
	}
	if ni.SubnetID != "" {
		parts = append(parts, ni.SubnetID)
	}

	return strings.Join(parts, " ")
}

func instanceMetricsBlock(o *aws.InstanceOverview) string {
	if err := o.Err(aws.SectionMetrics); err != nil {
		return sectionUnavailable("Metrics", err)
	}
	m := o.Metrics

	percent := func(v float64) string { return fmt.Sprintf("%.1f%%", v) }
	count := func(v float64) string { return strconv.Itoa(int(v)) }

	// The CPU row gets the bar the mockups draw; the byte metrics have no 0..100 scale for one.
	cpu := MetricReading(m.CPUUtilization, "5-min avg", percent)
	if m.CPUUtilization.OK {
		cpu = Gauge(ecsGaugeWidth, m.CPUUtilization.Value) + fmt.Sprintf("  (5-min avg @ %s)", m.CPUUtilization.At.UTC().Format("15:04Z"))
	}

	return SectionTitle("Metrics") + "\n" + kvBlock([]kv{
		{"CPU", cpu},
		{"Network in", MetricReading(m.NetworkIn, "5-min total", FormatByteCount)},
		{"Network out", MetricReading(m.NetworkOut, "5-min total", FormatByteCount)},
		{"Disk read", MetricReading(m.DiskReadBytes, "5-min total", FormatByteCount)},
		{"Disk write", MetricReading(m.DiskWriteBytes, "5-min total", FormatByteCount)},
		{"Status check", MetricReading(m.StatusCheckFailed, "5-min max", count)},
	})
}

func instanceStatusBlock(o *aws.InstanceOverview) string {
	if err := o.Err(aws.SectionStatus); err != nil {
		return sectionUnavailable("Status", err)
	}
	s := o.Status

	lines := []string{SectionTitle("Status"), kvBlock([]kv{
		{"State", Badge(s.InstanceState)},
		{"System", Badge(s.SystemStatus)},
		{"Instance", Badge(s.InstanceStatus)},
	})}

	lines = append(lines, "Scheduled events:")
	if len(s.ScheduledEvents) == 0 {
		lines = append(lines, "  none")
	}
	for _, event := range s.ScheduledEvents {
		line := "  " + utils.ColoredString(event.Code, color.FgYellow) + " " + event.NotBefore
		if event.NotAfter != "" {
			line += " - " + event.NotAfter
		}
		if event.Description != "" {
			line += " " + utils.ColoredString(event.Description, color.Faint)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n") + "\n" + instanceAlarmsLines(o) + "\n" + instanceASGLines(o)
}

// pendingValue is what a selection-time field says on the pane's first paint, before its fetch has answered: an ellipsis, never a value the fetch has not verified.
func pendingValue() string {
	return utils.ColoredString("…", color.Faint)
}

// instanceAlarmsLines and instanceASGLines sit inside Status rather than in sections of their own: both are one line on almost every instance, and a heading per line turns the column into a list of headings.
func instanceAlarmsLines(o *aws.InstanceOverview) string {
	if err := o.Err(aws.SectionAlarms); err != nil {
		return "Alarms: " + utils.ColoredString("unavailable", color.FgRed)
	}
	if o.ExtrasPending {
		return "Alarms: " + pendingValue()
	}
	if len(o.Alarms) == 0 {
		return "Alarms:\n  none"
	}

	lines := []string{"Alarms:"}
	for _, alarm := range o.Alarms {
		lines = append(lines, "  "+Badge(alarm.State)+" "+alarm.Name+" "+utils.ColoredString(alarm.MetricName, color.Faint))
	}

	return strings.Join(lines, "\n")
}

func instanceASGLines(o *aws.InstanceOverview) string {
	if err := o.Err(aws.SectionASG); err != nil {
		return "Auto Scaling: " + utils.ColoredString("unavailable", color.FgRed)
	}
	if o.ExtrasPending {
		return "Auto Scaling: " + pendingValue()
	}
	// A nil membership is the answer for an instance that belongs to no group, not a missing read: GetInstanceASGMembership reports that case as nil with no error.
	if o.ASG == nil {
		return "Auto Scaling: none"
	}

	return fmt.Sprintf("Auto Scaling: %s (desired %d, min %d, max %d)", o.ASG.GroupName, o.ASG.Desired, o.ASG.Min, o.ASG.Max)
}

// storageVolumeIDWidth is the table width at which the volume-id column joins: the widest realistic row with it stays whole there. A tune-by-eye number like minTwoColWidth, not a derived one.
const storageVolumeIDWidth = 80

func instanceStorageBlock(o *aws.InstanceOverview, width int) string {
	if err := o.Err(aws.SectionDetails); err != nil {
		return sectionUnavailable("Storage", err)
	}

	title := SectionTitle("Storage")
	devices := o.Details.BlockDevices
	if len(devices) == 0 {
		return title + "\nno EBS volumes"
	}

	// gp2 volumes report no throughput at all, so the column is carried only when some volume actually has one: an unconditional "0 MiB/s" is a reading nobody published.
	throughput := slices.ContainsFunc(devices, func(d aws.BlockDevice) bool { return d.Throughput > 0 })
	// The volume id is 21 cells the narrow pane cannot spare without cutting the encryption flag, which is the one column anybody acts on; wide panes carry it, narrow ones keep the row whole.
	withVolume := width >= storageVolumeIDWidth

	rows := make([][]utils.Cell, len(devices))
	for i, d := range devices {
		cells := []utils.Cell{{Text: d.DeviceName}}
		if withVolume {
			cells = append(cells, utils.Cell{Text: d.VolumeID, Color: color.Faint})
		}
		cells = append(cells,
			utils.Cell{Text: fmt.Sprintf("%d GiB", d.VolumeSize)},
			utils.Cell{Text: d.VolumeType},
			utils.Cell{Text: fmt.Sprintf("%d IOPS", d.Iops)},
		)
		if throughput {
			cells = append(cells, utils.Cell{Text: fmt.Sprintf("%d MiB/s", d.Throughput)})
		}
		rows[i] = append(cells, encryptionCell(d.Encrypted))
	}

	// Every column holds a value of its own natural width, so none takes a weight; the rows and the weights are built together, which is why neither error RenderTableFit reports can happen.
	table, _ := utils.RenderTableFit(rows, width, make([]int, len(rows[0])))

	return title + "\n" + table + "\n" + instanceSnapshotLines(o)
}

// instanceSnapshotLines rides the selection-time schedule (see ec2OverviewExtras), so on the first render of a selection the list can be legitimately absent without having failed.
func instanceSnapshotLines(o *aws.InstanceOverview) string {
	if err := o.Err(aws.SectionSnapshots); err != nil {
		return "Snapshots: " + utils.ColoredString("unavailable", color.FgRed)
	}
	if o.ExtrasPending {
		return "Snapshots: " + pendingValue()
	}
	if len(o.Snapshots) == 0 {
		return "Snapshots: none"
	}

	lines := []string{"Snapshots:"}
	for _, s := range o.Snapshots {
		lines = append(lines, fmt.Sprintf("  %s %s %s %s (%d GiB) started %s", s.SnapshotID, utils.ColoredString(s.VolumeID, color.Faint), s.State, s.Progress, s.SizeGiB, s.StartTime))
	}

	return strings.Join(lines, "\n")
}

// instanceConsoleBlock reports availability and age only: the payloads stay behind the actions menu, and the age matters because AWS captures the console log at boot and never again.
func instanceConsoleBlock(o *aws.InstanceOverview, now time.Time) string {
	if err := o.Err(aws.SectionConsole); err != nil {
		return sectionUnavailable("Console", err)
	}
	c := o.Console
	if c == nil {
		if o.ExtrasPending {
			return SectionTitle("Console") + "\n" + pendingValue()
		}
		return SectionTitle("Console") + "\nnot fetched yet"
	}

	output := "none"
	if c.OutputBytes > 0 {
		output = fmt.Sprintf("available · %s · captured %s", FormatByteCount(float64(c.OutputBytes)), RelTime(c.CapturedAt, now))
	}
	screenshot := "none"
	if c.ScreenshotBytes > 0 {
		screenshot = "available · " + FormatByteCount(float64(c.ScreenshotBytes))
	}

	return SectionTitle("Console") + "\n" + kvBlock([]kv{
		{"Output", output},
		{"Screenshot", screenshot},
	})
}

// encryptionCell says which state it is in words, because this table has no header row and a bare "no" between a volume type and an IOPS figure says nothing.
// Amber marks only the unencrypted case: encryption at rest is the expected posture, and colouring both spends the reader's attention on the one that needs none.
func encryptionCell(encrypted bool) utils.Cell {
	if encrypted {
		return utils.Cell{Text: "encrypted"}
	}

	return utils.Cell{Text: "unencrypted", Color: color.FgYellow}
}

func instanceSecurityBlock(o *aws.InstanceOverview) string {
	if err := o.Err(aws.SectionDetails); err != nil {
		return sectionUnavailable("Security", err)
	}

	lines := []string{SectionTitle("Security")}
	if len(o.Details.SecurityGroups) == 0 {
		lines = append(lines, "no security groups")
	}
	for _, sg := range o.Details.SecurityGroups {
		lines = append(lines, orNone(sg.Name)+"  "+utils.ColoredString(sg.ID, color.Faint))
	}

	return strings.Join(lines, "\n")
}

func instanceTagsBlock(o *aws.InstanceOverview, width int) string {
	if err := o.Err(aws.SectionDetails); err != nil {
		return sectionUnavailable("Tags", err)
	}
	if len(o.Details.Tags) == 0 {
		return SectionTitle("Tags") + "\nnone"
	}

	tags := make([]kv, len(o.Details.Tags))
	for i, tag := range o.Details.Tags {
		tags[i] = kv{tag.Key, tag.Value}
	}

	return SectionTitle("Tags") + "\n" + tagsBody(width, tags)
}
