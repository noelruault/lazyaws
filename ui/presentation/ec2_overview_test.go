package presentation

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

func overviewInstance() *aws.Instance {
	return &aws.Instance{
		ID:           "i-0abcdef1234567890",
		Name:         "web-1",
		State:        "running",
		InstanceType: "t3a.micro",
		AZ:           "eu-west-1a",
	}
}

// fullOverview is an instance with every section answered, so a test that removes one thing is testing that one thing.
func fullOverview() *aws.InstanceOverview {
	return &aws.InstanceOverview{
		Details: &aws.InstanceDetails{
			Instance: aws.Instance{
				ID:           "i-0abcdef1234567890",
				Name:         "web-1",
				State:        "running",
				InstanceType: "t3a.micro",
				AZ:           "eu-west-1a",
				PrivateIP:    "198.51.100.178",
				PublicIP:     "203.0.113.10",
				Tags:         []aws.Tag{{Key: "Env", Value: "prod"}},
			},
			LaunchTime:         "2026-08-20T09:00:00Z",
			VpcID:              "vpc-0abcdef1234567890",
			SubnetID:           "subnet-0abcdef1234567890",
			KeyName:            "web-kp",
			Architecture:       "x86_64",
			Platform:           "Linux/UNIX",
			RootDeviceType:     "ebs",
			Monitoring:         "disabled",
			IamInstanceProfile: "web-role",
			SecurityGroups:     []aws.SecurityGroup{{ID: "sg-0abcdef1234567890", Name: "web-sg"}},
			BlockDevices: []aws.BlockDevice{
				{DeviceName: "/dev/sda1", VolumeID: "vol-0fedcba9876543210", VolumeSize: 8, VolumeType: "gp2", Iops: 100},
			},
			NetworkInterfaces: []aws.NetworkInterface{{ID: "eni-0abcdef1234567890", PrivateIP: "198.51.100.178"}},
			InstanceTypeInfo:  &aws.InstanceTypeInfo{VCpus: 2, Memory: 1024, NetworkPerformance: "Up to 5 Gigabit"},
			ElasticIPs:        []aws.ElasticIP{{PublicIP: "203.0.113.10"}},
		},
		Status: &aws.InstanceStatus{
			InstanceState:    "running",
			SystemStatus:     "ok",
			InstanceStatus:   "ok",
			SystemStatusOk:   true,
			InstanceStatusOk: true,
			ScheduledEvents:  []aws.ScheduledEvent{{Code: "instance-reboot", NotBefore: "2026-09-01"}},
		},
		Metrics: &aws.InstanceMetrics{
			CPUUtilization: aws.MetricPoint{Value: 0.523, At: time.Date(2026, 8, 27, 11, 43, 0, 0, time.UTC), OK: true},
			NetworkIn:      aws.MetricPoint{Value: 240640, At: time.Date(2026, 8, 27, 11, 43, 0, 0, time.UTC), OK: true},
		},
		ASG:    &aws.ASGMembership{GroupName: "web-asg", Desired: 2, Min: 1, Max: 4},
		Alarms: []aws.InstanceAlarm{{Name: "cpu-high", State: "OK", MetricName: "CPUUtilization"}},
		Errs:   map[string]error{},
	}
}

// emptyOverview answers every section with the absence AWS actually reports: no tags, no groups, no interfaces, no EIPs, no events, no alarms, no volumes, and no ASG membership.
func emptyOverview() *aws.InstanceOverview {
	return &aws.InstanceOverview{
		Details: &aws.InstanceDetails{Instance: aws.Instance{ID: "i-1", InstanceType: "t2.micro"}},
		Status:  &aws.InstanceStatus{InstanceState: "stopped"},
		Metrics: &aws.InstanceMetrics{},
		Errs:    map[string]error{},
	}
}

// The first paint of a selection lands before the selection-time extras have answered, and every field they own must read "…" rather than an absence nothing has verified.
func TestInstanceOverviewPendingExtrasSaySoInsteadOfClaimingAbsences(t *testing.T) {
	forceColor(t)
	o := fullOverview()
	o.ExtrasPending = true
	o.ASG, o.Alarms, o.Console, o.Snapshots = nil, nil, nil, nil
	o.Details.ElasticIPs = nil

	got := kvPadding.ReplaceAllString(utils.Decolorise(FormatInstanceOverview(overviewInstance(), o, stackedWidth, overviewNow)), " ")

	for _, want := range []string{"Auto Scaling: …", "Snapshots: …", "Elastic IPs: …"} {
		if !strings.Contains(got, want) {
			t.Errorf("pending overview is missing %q\n%s", want, got)
		}
	}
	if got := instanceAlarmsCell(o).Text; got != "…" {
		t.Errorf("pending alarms card = %q, want %q", got, "…")
	}
	for _, absent := range []string{"Auto Scaling: none", "Snapshots: none", "not fetched yet"} {
		if strings.Contains(got, absent) {
			t.Errorf("pending overview claims %q before the fetch answered\n%s", absent, got)
		}
	}
}

func TestInstanceOverviewRendersEverySection(t *testing.T) {
	got := FormatInstanceOverview(overviewInstance(), fullOverview(), stackedWidth, overviewNow)

	for _, want := range []string{
		"Instance", "web-1", "i-0abcdef1234567890",
		// No State card: the header badge beside the name is the same field.
		"● running", "Checks", "2/2 ok", "Alarms", "1",
		"Configuration", "t3a.micro · 2 vCPU · 1.0 GiB", "eu-west-1a", "x86_64", "Linux/UNIX", "web-kp", "web-role",
		"Network", "198.51.100.178", "203.0.113.10", "vpc-0abcdef1234567890", "Up to 5 Gigabit", "eni-0abcdef1234567890",
		// The CPU row carries the mockups' bar; a reading of 0.5% fills no cells at this width, so the empty bar plus its number is the recorded render.
		"Metrics", "▕░░░░░░░░░░▏ 0.5%  (5-min avg @ 11:43Z)", "235.0 KiB (5-min total @ 11:43Z)",
		"Status", "System", "● ok", "Instance", "instance-reboot", "web-asg (desired 2, min 1, max 4)",
		"Storage", "Device", "Size", "Type", "IOPS", "Encrypted", "/dev/sda1", "8 GiB", "gp2", "100 IOPS", "unencrypted", "Snapshots: none",
		"Security", "web-sg", "sg-0abcdef1234567890",
		"Tags", "Env: prod",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview is missing %q\n%s", want, got)
		}
	}

	plain := utils.Decolorise(got)
	if header := strings.SplitN(plain, "\n\n", 2)[0]; strings.Count(header, "┌") != 2 {
		t.Errorf("header does not contain two stat cards\n%s", header)
	}
	_, statusAndAfter, found := strings.Cut(plain, "♡ Status\n")
	if !found {
		t.Fatalf("overview has no Status section\n%s", plain)
	}
	status, _, found := strings.Cut(statusAndAfter, "\n\n▣ Storage")
	if !found || strings.Count(status, "┌") != 2 {
		t.Errorf("Status does not contain three filled cards\n%s", status)
	}
	if strings.Count(plain, "├") != 1 {
		t.Errorf("overview does not contain one boxed storage table\n%s", plain)
	}
}

func TestInstanceOverviewCardsUseHealthColours(t *testing.T) {
	forceColor(t)

	healthy := FormatInstanceOverview(overviewInstance(), fullOverview(), stackedWidth, overviewNow)
	for _, want := range []string{
		utils.ColoredString("● running", color.FgGreen),
		utils.ColoredString("2/2 ok", color.FgGreen),
		utils.ColoredString("● ok", color.FgGreen),
		utils.ColoredString("1", color.FgRed),
	} {
		if !strings.Contains(healthy, want) {
			t.Errorf("healthy overview is missing coloured card value %q\n%s", utils.Decolorise(want), utils.Decolorise(healthy))
		}
	}

	failed := fullOverview()
	failed.Status.InstanceStatus = "impaired"
	failed.Status.InstanceStatusOk = false
	got := FormatInstanceOverview(overviewInstance(), failed, stackedWidth, overviewNow)
	if want := utils.ColoredString("1/2 failed", color.FgRed); !strings.Contains(got, want) {
		t.Errorf("failed checks card is missing %q in red\n%s", utils.Decolorise(want), utils.Decolorise(got))
	}

	if alarms := instanceAlarmsCell(emptyOverview()); alarms.Text != "0" || alarms.Color != 0 {
		t.Errorf("zero alarms card = %+v, want a plain zero", alarms)
	}
}

// The launch time carries its age, because uptime is what an operator reads it for and the bare stamp is what the Config tab already shows.
func TestInstanceOverviewDatesTheLaunchTime(t *testing.T) {
	got := FormatInstanceOverview(overviewInstance(), fullOverview(), stackedWidth, overviewNow)

	if want := "2026-08-20T09:00:00Z (7d ago)"; !strings.Contains(got, want) {
		t.Errorf("overview is missing %q\n%s", want, got)
	}
}

// An unparseable launch time degrades to the stamp itself: a wrong age is worse than no age.
func TestInstanceOverviewLaunchTimeSurvivesAnUnparseableStamp(t *testing.T) {
	if got := launchedAt("not a timestamp", overviewNow); got != "not a timestamp" {
		t.Errorf("launchedAt(unparseable) = %q, want the stamp passed through", got)
	}
	if got := launchedAt("", overviewNow); got != "none" {
		t.Errorf("launchedAt(\"\") = %q, want %q", got, "none")
	}
}

// Every list an instance can report empty says so, rather than leaving a heading with nothing under it.
func TestInstanceOverviewStatesEveryAbsence(t *testing.T) {
	got := utils.Decolorise(FormatInstanceOverview(overviewInstance(), emptyOverview(), stackedWidth, overviewNow))

	for _, want := range []string{
		"Interfaces:\n  none",
		"Elastic IPs:\n  none",
		"no EBS volumes",
		"no security groups",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview is missing %q\n%s", want, got)
		}
	}
	for _, want := range []*regexp.Regexp{
		regexp.MustCompile(`Scheduled events:\s+none`),
		regexp.MustCompile(`Auto Scaling:\s+none`),
	} {
		if !want.MatchString(got) {
			t.Errorf("overview is missing %q\n%s", want, got)
		}
	}
	if alarms := instanceAlarmsCell(emptyOverview()); alarms.Text != "0" {
		t.Errorf("empty overview alarms card = %q, want %q", alarms.Text, "0")
	}

	// Tags is the one absence that is a bare "none" under its heading, so it is asserted as the line it occupies rather than by Contains, which "none" would satisfy from any other section.
	if !strings.Contains(got, "Tags\nnone") {
		t.Errorf("overview does not state that there are no tags\n%s", got)
	}
}

func TestInstanceOverviewKeepsAlarmAndASGErrorsVisible(t *testing.T) {
	o := fullOverview()
	o.Errs[aws.SectionAlarms] = errors.New("alarm lookup failed: AccessDenied")
	o.Errs[aws.SectionASG] = errors.New("ASG lookup failed: ThrottlingException")

	got := kvPadding.ReplaceAllString(utils.Decolorise(FormatInstanceOverview(overviewInstance(), o, stackedWidth, overviewNow)), " ")
	for _, want := range []string{
		"Alarms: unavailable: alarm lookup failed: AccessDenied",
		"Auto Scaling: unavailable: ASG lookup failed: ThrottlingException",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview dropped fetch error %q\n%s", want, got)
		}
	}
}

// Network performance has no absent state: every instance type carries a rating, so a nil InstanceTypeInfo is the DescribeInstanceTypes lookup having failed and "none" was a statement about the instance that AWS never made.
// The row is read as a whole line rather than by Contains, because "none" is on half a dozen other lines of the same pane.
func TestInstanceOverviewRendersMissingNetworkPerformanceAsUnavailable(t *testing.T) {
	got := utils.Decolorise(FormatInstanceOverview(overviewInstance(), emptyOverview(), stackedWidth, overviewNow))

	line := ""
	for _, l := range strings.Split(got, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "Performance:") {
			line = strings.TrimSpace(l)
		}
	}
	if line == "" {
		t.Fatalf("overview has no performance row\n%s", got)
	}
	if !strings.Contains(line, "unavailable") || strings.Contains(line, "none") {
		t.Errorf("performance line = %q, want it to read unavailable and not none", line)
	}

	// The type name is still the honest answer to what the instance IS, and this row must not have taken it down.
	if !strings.Contains(got, "t2.micro") {
		t.Errorf("a failed instance-type lookup took the type name with it\n%s", got)
	}
}

// An instance in no Auto Scaling group is a nil membership with no error, and reading that as a failed lookup would report a broken permission on the commonest case there is.
func TestInstanceOverviewSeparatesNoASGFromAFailedASGLookup(t *testing.T) {
	absent := kvPadding.ReplaceAllString(utils.Decolorise(FormatInstanceOverview(overviewInstance(), emptyOverview(), stackedWidth, overviewNow)), " ")
	if !strings.Contains(absent, "Auto Scaling: none") {
		t.Errorf("a nil ASG membership should read as none\n%s", absent)
	}

	o := emptyOverview()
	o.Errs[aws.SectionASG] = errors.New("AccessDenied")
	failed := kvPadding.ReplaceAllString(utils.Decolorise(FormatInstanceOverview(overviewInstance(), o, stackedWidth, overviewNow)), " ")
	if !strings.Contains(failed, "Auto Scaling: unavailable") {
		t.Errorf("a failed ASG lookup should say so\n%s", failed)
	}
}

// A metric series that published nothing reads "no data": an EBS-only instance never publishes disk metrics, and 0 would be a reading nobody took.
func TestInstanceOverviewRendersAnUnpublishedMetricAsNoData(t *testing.T) {
	got := utils.Decolorise(FormatInstanceOverview(overviewInstance(), emptyOverview(), stackedWidth, overviewNow))

	if !strings.Contains(got, "Disk read:") {
		t.Fatalf("overview has no disk read row\n%s", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Disk read:") && !strings.Contains(line, "no data") {
			t.Errorf("disk read line = %q, want it to read no data", line)
		}
	}
}

func TestInstanceOverviewSectionsFailIndependently(t *testing.T) {
	tests := []struct {
		section string
		want    []string
		intact  string
	}{
		{section: aws.SectionDetails, want: []string{"Configuration", "Network", "Storage", "Security", "Tags"}, intact: "Metrics"},
		{section: aws.SectionMetrics, want: []string{"Metrics"}, intact: "Configuration"},
		{section: aws.SectionStatus, want: []string{"Status"}, intact: "Configuration"},
	}

	for _, tt := range tests {
		t.Run(tt.section, func(t *testing.T) {
			o := fullOverview()
			o.Errs[tt.section] = errors.New("AccessDenied: not authorized")
			got := utils.Decolorise(FormatInstanceOverview(overviewInstance(), o, stackedWidth, overviewNow))

			// Every section reading that fetch has to say it is unavailable, and say why: the overview retries on its own interval, so a throttle and a denial must not look alike.
			unavailable := strings.Count(got, "unavailable: AccessDenied: not authorized")
			if unavailable != len(tt.want) {
				t.Errorf("%d sections reported unavailable, want %d\n%s", unavailable, len(tt.want), got)
			}
			for _, section := range tt.want {
				if !strings.Contains(got, section+"\nunavailable: AccessDenied") {
					t.Errorf("section %q does not report its fetch failing\n%s", section, got)
				}
			}
			if !strings.Contains(got, tt.intact) {
				t.Errorf("section %q should have survived a %q failure\n%s", tt.intact, tt.section, got)
			}
		})
	}
}

func TestInstanceOverviewReportsAStatusFetchThatReturnedNothing(t *testing.T) {
	o := fullOverview()
	o.Status = nil

	got := utils.Decolorise(FormatInstanceOverview(overviewInstance(), o, stackedWidth, overviewNow))
	if want := "Status\nunavailable: instance status not returned"; !strings.Contains(got, want) {
		t.Errorf("overview does not report missing status data\n%s", got)
	}
}

// The header is built from the list row, so an instance whose every fetch failed is still identified instead of leaving an anonymous pane of errors.
func TestInstanceOverviewHeaderSurvivesEverySectionFailing(t *testing.T) {
	o := &aws.InstanceOverview{Errs: map[string]error{
		aws.SectionDetails: errors.New("boom"),
		aws.SectionStatus:  errors.New("boom"),
		aws.SectionMetrics: errors.New("boom"),
	}}

	got := utils.Decolorise(FormatInstanceOverview(overviewInstance(), o, stackedWidth, overviewNow))

	for _, want := range []string{"web-1", "i-0abcdef1234567890", "t3a.micro"} {
		if !strings.Contains(got, want) {
			t.Errorf("header is missing %q when every section failed\n%s", want, got)
		}
	}
}

func TestInstanceOverviewHeaderNamesAnUnnamedInstance(t *testing.T) {
	inst := overviewInstance()
	inst.Name = ""

	got := utils.Decolorise(FormatInstanceOverview(inst, emptyOverview(), stackedWidth, overviewNow))
	if !strings.Contains(got, "(no name)") {
		t.Errorf("an unnamed instance should be labelled\n%s", got)
	}
}

// Encryption is the one thing on the storage row that needs acting on, so it says which state it is in words and marks only the state that needs attention.
func TestInstanceOverviewStorageMarksUnencryptedVolumes(t *testing.T) {
	forceColor(t)

	unencrypted := encryptionCell(false)
	if unencrypted.Text != "unencrypted" {
		t.Errorf("unencrypted cell text = %q, want it to name its own state", unencrypted.Text)
	}
	if !strings.Contains(unencrypted.Rendered(), "\x1b[33m") {
		t.Errorf("unencrypted cell = %q, want it amber", unencrypted.Rendered())
	}

	encrypted := encryptionCell(true)
	if encrypted.Text != "encrypted" {
		t.Errorf("encrypted cell text = %q", encrypted.Text)
	}
	if strings.Contains(encrypted.Rendered(), "\x1b[33m") {
		t.Errorf("encrypted cell = %q, want no amber on the posture that needs no attention", encrypted.Rendered())
	}
}

func TestInstanceOverviewStorageTableKeepsEveryColumn(t *testing.T) {
	forceColor(t)

	o := fullOverview()
	o.Details.BlockDevices = append(o.Details.BlockDevices, aws.BlockDevice{
		DeviceName: "/dev/xvdba", VolumeID: "vol-0abcdef1234567890", VolumeSize: 16384, VolumeType: "io2", Iops: 64000, Throughput: 4000, Encrypted: true,
	})

	headerColumns := regexp.MustCompile(`Device\s+Size\s+Type\s+IOPS\s+Encrypted`)
	gp2Columns := regexp.MustCompile(`/dev/sda1\s+8 GiB\s+gp2\s+100 IOPS\s+unencrypted`)
	io2Columns := regexp.MustCompile(`/dev/xvdba\s+16384 GiB\s+io2\s+64000 IOPS\s+encrypted`)
	for _, width := range []int{80, 110, 120, 160} {
		got := utils.Decolorise(FormatInstanceOverview(overviewInstance(), o, width, overviewNow))

		header := lineContaining(got, "Device")
		if header == "" || !headerColumns.MatchString(header) {
			t.Errorf("at width %d storage header lost a column: %q\n%s", width, header, got)
		}
		gp2 := lineContaining(got, "/dev/sda1")
		if gp2 == "" || !gp2Columns.MatchString(gp2) {
			t.Errorf("at width %d gp2 row lost a column: %q\n%s", width, gp2, got)
		}
		io2 := lineContaining(got, "/dev/xvdba")
		if io2 == "" || !io2Columns.MatchString(io2) {
			t.Errorf("at width %d io2 row lost a column: %q\n%s", width, io2, got)
		}
	}
}

// Sections inside a column are separated by a blank line: without one, the last row of a section and the next heading read as a single list.
func TestInstanceOverviewSeparatesItsSections(t *testing.T) {
	got := utils.Decolorise(FormatInstanceOverview(overviewInstance(), fullOverview(), stackedWidth, overviewNow))

	for _, want := range []string{"\n\n⇄ Network\n", "\n\n◒ Metrics\n", "\n\n▣ Storage\n", "\n\n⌾ Security\n", "\n\n◇ Tags\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("section %q is not preceded by a blank line\n%s", strings.TrimSpace(want), got)
		}
	}
}

// ColumnWidth is what the storage table is built for and Columns is what cuts it; if they disagree the table is either truncated or leaves the column short.
func TestColumnWidthAgreesWithColumns(t *testing.T) {
	for width := 20; width <= 240; width++ {
		for _, gap := range []int{0, 1, 2, 4} {
			column := ColumnWidth(width, gap)

			// A left block of solid cells is padded to exactly the column width, so the rule's position reports what Columns really allotted.
			left := strings.Repeat("x", column)
			line := strings.Split(Columns(width, gap, left, "right"), "\n")[0]

			if column == width {
				if strings.Contains(line, "│") {
					t.Fatalf("ColumnWidth(%d, %d) reported a stack, but Columns rendered two columns: %q", width, gap, line)
				}
				continue
			}
			if got := runewidth.StringWidth(strings.Split(line, "│")[0]) - gap; got != column {
				t.Fatalf("Columns(%d, %d) gave the left column %d cells, ColumnWidth said %d", width, gap, got, column)
			}
		}
	}
}

// Wrapping is off in main, so a line wider than the pane is clipped at the edge with nothing to say it was clipped.
func TestInstanceOverviewNeverExceedsTheWidth(t *testing.T) {
	overviews := map[string]*aws.InstanceOverview{"full": fullOverview(), "empty": emptyOverview()}
	failed := fullOverview()
	failed.Errs[aws.SectionDetails] = errors.New("AccessDenied: not authorized to perform ec2:DescribeInstances")
	overviews["failed"] = failed

	// The header spans the pane rather than a column, so Columns never measures it: a long Name tag beside the badge and the instance id is what pushes it over on a narrow terminal.
	longName := overviewInstance()
	longName.Name = "prod-web-server-eu-west-1a-blue-green-canary"

	instances := map[string]*aws.Instance{"named": overviewInstance(), "long name": longName}

	for name, o := range overviews {
		for instName, inst := range instances {
			t.Run(name+"/"+instName, func(t *testing.T) {
				for width := 40; width <= 240; width++ {
					out := FormatInstanceOverview(inst, o, width, overviewNow)
					for _, line := range strings.Split(out, "\n") {
						if got := runewidth.StringWidth(utils.Decolorise(line)); got > width {
							t.Fatalf("width %d: line of %d cells: %q", width, got, utils.Decolorise(line))
						}
					}
				}
			})
		}
	}
}

// The Elastic IP list is fetched separately from the rest of the details, so a failed address lookup has to say so: rendering it as "none" would report an instance with no Elastic IP, which is a different fact.
func TestInstanceOverviewSeparatesNoElasticIPFromAFailedLookup(t *testing.T) {
	absent := utils.Decolorise(FormatInstanceOverview(overviewInstance(), emptyOverview(), stackedWidth, overviewNow))
	if !strings.Contains(absent, "Elastic IPs:\n  none") {
		t.Errorf("an instance with no Elastic IP should read none\n%s", absent)
	}

	o := emptyOverview()
	o.Errs[aws.SectionEIP] = errors.New("AccessDenied")
	failed := utils.Decolorise(FormatInstanceOverview(overviewInstance(), o, stackedWidth, overviewNow))
	if !strings.Contains(failed, "Elastic IPs: unavailable") {
		t.Errorf("a failed address lookup should say so\n%s", failed)
	}
	if strings.Contains(failed, "Elastic IPs:\n  none") {
		t.Errorf("a failed address lookup must not read as an absence\n%s", failed)
	}
}
