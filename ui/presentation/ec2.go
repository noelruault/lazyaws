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
	header := ResourceHeader("Instance", instanceName(inst), Badge(inst.State), inst.ID, inst.InstanceType, inst.AZ)

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
		instanceTagsBlock(o),
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

	performance := "none"
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
		lines = append(lines, "  "+ni.ID+" "+utils.ColoredString(orNone(ni.PrivateIP), color.Faint))
	}

	lines = append(lines, "Elastic IPs:")
	if len(d.ElasticIPs) == 0 {
		lines = append(lines, "  none")
	}
	for _, eip := range d.ElasticIPs {
		lines = append(lines, "  "+eip.PublicIP)
	}

	return strings.Join(lines, "\n")
}

func instanceMetricsBlock(o *aws.InstanceOverview) string {
	if err := o.Err(aws.SectionMetrics); err != nil {
		return sectionUnavailable("Metrics", err)
	}
	m := o.Metrics

	percent := func(v float64) string { return fmt.Sprintf("%.1f%%", v) }
	count := func(v float64) string { return strconv.Itoa(int(v)) }

	return SectionTitle("Metrics") + "\n" + kvBlock([]kv{
		{"CPU", MetricReading(m.CPUUtilization, "5-min avg", percent)},
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
		lines = append(lines, "  "+utils.ColoredString(event.Code, color.FgYellow)+" "+event.NotBefore)
	}

	return strings.Join(lines, "\n") + "\n" + instanceAlarmsLines(o) + "\n" + instanceASGLines(o)
}

// instanceAlarmsLines and instanceASGLines sit inside Status rather than in sections of their own: both are one line on almost every instance, and a heading per line turns the column into a list of headings.
func instanceAlarmsLines(o *aws.InstanceOverview) string {
	if err := o.Err(aws.SectionAlarms); err != nil {
		return "Alarms: " + utils.ColoredString("unavailable", color.FgRed)
	}
	if len(o.Alarms) == 0 {
		return "Alarms:\n  none"
	}

	lines := []string{"Alarms:"}
	for _, alarm := range o.Alarms {
		lines = append(lines, "  "+Badge(alarm.State)+" "+alarm.Name)
	}

	return strings.Join(lines, "\n")
}

func instanceASGLines(o *aws.InstanceOverview) string {
	if err := o.Err(aws.SectionASG); err != nil {
		return "Auto Scaling: " + utils.ColoredString("unavailable", color.FgRed)
	}
	// A nil membership is the answer for an instance that belongs to no group, not a missing read: GetInstanceASGMembership reports that case as nil with no error.
	if o.ASG == nil {
		return "Auto Scaling: none"
	}

	return fmt.Sprintf("Auto Scaling: %s (desired %d, min %d, max %d)", o.ASG.GroupName, o.ASG.Desired, o.ASG.Min, o.ASG.Max)
}

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

	rows := make([][]utils.Cell, len(devices))
	for i, d := range devices {
		cells := []utils.Cell{
			{Text: d.DeviceName},
			{Text: fmt.Sprintf("%d GiB", d.VolumeSize)},
			{Text: d.VolumeType},
			{Text: fmt.Sprintf("%d IOPS", d.Iops)},
		}
		if throughput {
			cells = append(cells, utils.Cell{Text: fmt.Sprintf("%d MiB/s", d.Throughput)})
		}
		rows[i] = append(cells, encryptionCell(d.Encrypted))
	}

	// Every column holds a value of its own natural width, so none takes a weight; the rows and the weights are built together, which is why neither error RenderTableFit reports can happen.
	table, _ := utils.RenderTableFit(rows, width, make([]int, len(rows[0])))

	return title + "\n" + table
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

func instanceTagsBlock(o *aws.InstanceOverview) string {
	if err := o.Err(aws.SectionDetails); err != nil {
		return sectionUnavailable("Tags", err)
	}

	lines := []string{SectionTitle("Tags")}
	if len(o.Details.Tags) == 0 {
		lines = append(lines, "none")
	}
	for _, tag := range o.Details.Tags {
		lines = append(lines, tag.Key+": "+orNone(tag.Value))
	}

	return strings.Join(lines, "\n")
}
