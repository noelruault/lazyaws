package presentation

import (
	"errors"
	"strings"
	"testing"
	"time"

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
			InstanceState:   "running",
			SystemStatus:    "ok",
			InstanceStatus:  "ok",
			ScheduledEvents: []aws.ScheduledEvent{{Code: "instance-reboot", NotBefore: "2026-09-01"}},
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

func TestInstanceOverviewRendersEverySection(t *testing.T) {
	got := FormatInstanceOverview(overviewInstance(), fullOverview(), stackedWidth, overviewNow)

	for _, want := range []string{
		"Instance", "web-1", "i-0abcdef1234567890",
		"Configuration", "t3a.micro · 2 vCPU · 1.0 GiB", "eu-west-1a", "x86_64", "Linux/UNIX", "web-kp", "web-role",
		"Network", "198.51.100.178", "203.0.113.10", "vpc-0abcdef1234567890", "Up to 5 Gigabit", "eni-0abcdef1234567890",
		// The CPU row carries the mockups' bar; a reading of 0.5% fills no cells at this width, so the empty bar plus its number is the recorded render.
		"Metrics", "▕░░░░░░░░░░▏ 0.5%  (5-min avg @ 11:43Z)", "235.0 KiB (5-min total @ 11:43Z)",
		"Status", "instance-reboot", "cpu-high", "web-asg (desired 2, min 1, max 4)",
		"Storage", "/dev/sda1", "8 GiB", "gp2", "100 IOPS", "unencrypted",
		"Security", "web-sg", "sg-0abcdef1234567890",
		"Tags", "Env: prod",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview is missing %q\n%s", want, got)
		}
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
	got := FormatInstanceOverview(overviewInstance(), emptyOverview(), stackedWidth, overviewNow)

	for _, want := range []string{
		"Interfaces:\n  none",
		"Elastic IPs:\n  none",
		"Scheduled events:\n  none",
		"Alarms:\n  none",
		"Auto Scaling: none",
		"no EBS volumes",
		"no security groups",
	} {
		if !strings.Contains(utils.Decolorise(got), want) {
			t.Errorf("overview is missing %q\n%s", want, got)
		}
	}

	// Tags is the one absence that is a bare "none" under its heading, so it is asserted as the line it occupies rather than by Contains, which "none" would satisfy from any other section.
	if !strings.Contains(utils.Decolorise(got), "Tags\nnone") {
		t.Errorf("overview does not state that there are no tags\n%s", got)
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
	absent := FormatInstanceOverview(overviewInstance(), emptyOverview(), stackedWidth, overviewNow)
	if !strings.Contains(utils.Decolorise(absent), "Auto Scaling: none") {
		t.Errorf("a nil ASG membership should read as none\n%s", absent)
	}

	o := emptyOverview()
	o.Errs[aws.SectionASG] = errors.New("AccessDenied")
	failed := FormatInstanceOverview(overviewInstance(), o, stackedWidth, overviewNow)
	if !strings.Contains(utils.Decolorise(failed), "Auto Scaling: unavailable") {
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

// Throughput is a gp3 field: gp2 volumes report none, and a column of "0 MiB/s" would be a reading nobody published.
func TestInstanceOverviewStorageCarriesThroughputOnlyWhenSomeVolumeHasIt(t *testing.T) {
	gp2Only := fullOverview()
	if got := utils.Decolorise(instanceStorageBlock(gp2Only, 80)); strings.Contains(got, "MiB/s") {
		t.Errorf("storage table carries a throughput column no volume filled\n%s", got)
	}

	withGP3 := fullOverview()
	withGP3.Details.BlockDevices = append(withGP3.Details.BlockDevices, aws.BlockDevice{
		DeviceName: "/dev/sdb", VolumeID: "vol-0abcdef1234567890", VolumeSize: 8, VolumeType: "gp3", Iops: 3000, Throughput: 125,
	})
	got := utils.Decolorise(instanceStorageBlock(withGP3, 80))
	if !strings.Contains(got, "125 MiB/s") {
		t.Errorf("storage table dropped the throughput a volume reports\n%s", got)
	}
	// The gp2 volume shares the column once it exists, and it has nothing to put in it.
	if !strings.Contains(got, "0 MiB/s") {
		t.Errorf("a volume with no throughput should still occupy the column\n%s", got)
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

// The encryption flag is the last column and RenderTableFit spends its budget left to right, so it is the one a miscalculated width eats.
// Contains is not enough to see that: a cut row still contains the prefix it was asked about, so this asserts the BOUNDARY — every storage row ends on a whole flag.
func TestInstanceOverviewStorageKeepsEncryptionAtTheTwoColumnThreshold(t *testing.T) {
	o := fullOverview()
	o.Details.BlockDevices = append(o.Details.BlockDevices, aws.BlockDevice{
		DeviceName: "/dev/sdb", VolumeID: "vol-0abcdef1234567890", VolumeSize: 8, VolumeType: "gp3", Iops: 3000, Throughput: 125,
	})

	rows := storageRows(t, utils.Decolorise(FormatInstanceOverview(overviewInstance(), o, minTwoColWidth, overviewNow)))
	if len(rows) != 2 {
		t.Fatalf("expected 2 storage rows, got %d: %q", len(rows), rows)
	}
	for _, row := range rows {
		if !strings.HasSuffix(row, "unencrypted") {
			t.Errorf("storage row does not end on a whole encryption flag: %q", row)
		}
	}
}

// The widest row EBS can produce does NOT fit the narrowest two-column pane, and this pins what it degrades to rather than leaving it to be discovered.
// The flag is cut, but the cut lands after the letters that tell the two states apart and the amber survives it, so the row still reports the one thing on it anybody acts on.
func TestInstanceOverviewStorageDegradesTheWidestVolumeReadably(t *testing.T) {
	forceColor(t)

	o := fullOverview()
	o.Details.BlockDevices = []aws.BlockDevice{
		// io2 Block Express, at the service's documented ceilings for size, IOPS and throughput.
		{DeviceName: "/dev/xvdba", VolumeSize: 16384, VolumeType: "io2", Iops: 64000, Throughput: 4000, Encrypted: true},
		{DeviceName: "/dev/sda1", VolumeSize: 8, VolumeType: "gp2", Iops: 100},
	}

	out := FormatInstanceOverview(overviewInstance(), o, minTwoColWidth, overviewNow)
	rows := storageRows(t, utils.Decolorise(out))
	if len(rows) != 2 {
		t.Fatalf("expected 2 storage rows, got %d: %q", len(rows), rows)
	}
	if !strings.HasSuffix(rows[0], "encr…") || !strings.HasSuffix(rows[1], "unen…") {
		t.Errorf("the widest volume's rows degraded differently than recorded: %q", rows)
	}
	if !strings.Contains(out, "\x1b[33munen") {
		t.Errorf("a cut unencrypted flag lost its amber, which is what carries it once the word is truncated:\n%s", out)
	}
}

// storageRows returns the device rows of a rendered overview, which on a stacked or zipped pane are the lines under the Storage heading.
func storageRows(t *testing.T, plain string) []string {
	t.Helper()

	var rows []string
	for _, line := range strings.Split(plain, "\n") {
		// Above minTwoColWidth the storage block sits in the right column, so each line still carries the left column and the rule.
		if _, right, found := strings.Cut(line, "│"); found {
			line = right
		}
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "/dev/") {
			rows = append(rows, line)
		}
	}

	return rows
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
