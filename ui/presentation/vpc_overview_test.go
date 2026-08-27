package presentation

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// plainVPC renders the overview as the words it chose: escapes stripped and the alignment padding collapsed, since neither is what these states are about.
func plainVPC(v *aws.VPC, o *aws.VPCOverview, width int) string {
	return kvPadding.ReplaceAllString(utils.Decolorise(FormatVPCOverview(v, o, width)), " ")
}

func overviewVPC() *aws.VPC {
	return &aws.VPC{
		ID:             "vpc-0abcdef1234567890",
		Name:           "app-core",
		State:          "available",
		CIDR:           "198.51.100.0/24",
		Tenancy:        "default",
		OwnerID:        "123456789012",
		DHCPOptionsID:  "dopt-0c1f2e3d",
		SecondaryCIDRs: []string{"10.70.0.0/16"},
		Tags:           []aws.Tag{{Key: "Env", Value: "prod"}},
	}
}

// fullVPCOverview answers every fetch, so a test that removes one thing is testing that one thing.
func fullVPCOverview() *aws.VPCOverview {
	return &aws.VPCOverview{
		DNSSupport:   true,
		DNSHostnames: true,
		Subnets: []aws.Subnet{
			{ID: "subnet-a", AZ: "eu-west-1a", Public: true, AvailableIPs: 250},
			{ID: "subnet-b", AZ: "eu-west-1b", Public: true, AvailableIPs: 250},
			{ID: "subnet-c", AZ: "eu-west-1a", AvailableIPs: 500},
			{ID: "subnet-d", AZ: "eu-west-1c", AvailableIPs: 500},
		},
		InternetGateways: []aws.InternetGateway{{ID: "igw-0f1e2d3c", State: "available"}},
		NATGateways: []aws.NATGateway{
			{ID: "nat-01", State: "available"},
			{ID: "nat-02", State: "pending"},
		},
		Endpoints: []aws.VPCEndpoint{
			{ID: "vpce-1", Type: "Interface", ServiceName: "com.amazonaws.eu-west-1.secretsmanager"},
			{ID: "vpce-2", Type: "Gateway", ServiceName: "com.amazonaws.eu-west-1.s3"},
		},
		Errs: map[string]error{},
	}
}

// emptyVPCOverview is a VPC with nothing attached: no subnets, no gateways, no endpoints, and DNS switched off.
func emptyVPCOverview() *aws.VPCOverview {
	return &aws.VPCOverview{Errs: map[string]error{}}
}

func TestVPCOverviewRendersEverySection(t *testing.T) {
	got := plainVPC(overviewVPC(), fullVPCOverview(), stackedWidth)

	for _, want := range []string{
		"VPC", "app-core", "vpc-0abcdef1234567890", "198.51.100.0/24",
		"Configuration", "Secondary CIDRs: 10.70.0.0/16", "Default: no", "Tenancy: default", "DHCP options: dopt-0c1f2e3d",
		"DNS", "Resolution: on", "Hostnames: on",
		"Subnets", "4 subnets · 2 public / 2 private", "1500 addresses free", "AZs: eu-west-1a (2), eu-west-1b (1), eu-west-1c (1)",
		"Gateways", "Internet gateway: igw-0f1e2d3c", "NAT gateways: 2 · available (1), pending (1)",
		"Endpoints", "2 endpoints · Gateway (1), Interface (1)", "s3, secretsmanager",
		"Tags", "Env: prod",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview is missing %q\n%s", want, got)
		}
	}
}

// The default VPC is the one every account has and the one an unexpected resource usually turns out to be in, so it is called out rather than left to a boolean row.
func TestVPCOverviewMarksTheDefaultVPC(t *testing.T) {
	vpc := overviewVPC()
	vpc.IsDefault = true

	got := plainVPC(vpc, fullVPCOverview(), stackedWidth)
	if !strings.Contains(got, "default VPC") {
		t.Errorf("overview does not mark the default VPC in its header\n%s", got)
	}
	if !strings.Contains(got, "Default: yes") {
		t.Errorf("overview does not report the default flag\n%s", got)
	}
}

// A VPC with nothing attached says so per section, rather than leaving four headings with nothing under them.
func TestVPCOverviewStatesEveryAbsence(t *testing.T) {
	got := plainVPC(&aws.VPC{ID: "vpc-empty", State: "available"}, emptyVPCOverview(), stackedWidth)

	for _, want := range []string{
		"(no name)",
		"CIDR: none",
		"Secondary CIDRs: none",
		"IPv6 CIDRs: none",
		"Subnets\nnone",
		"Internet gateway: none",
		"NAT gateways: none",
		"Endpoints\nnone",
		"Tags\nnone",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview is missing %q\n%s", want, got)
		}
	}
}

// Both DNS attributes default to on and are switched off deliberately, so an unreadable attribute must not render as the disabled state: they are three answers, not two.
func TestVPCOverviewSeparatesDNSOffFromDNSUnreadable(t *testing.T) {
	off := plainVPC(overviewVPC(), emptyVPCOverview(), stackedWidth)
	if !strings.Contains(off, "Resolution: off") || !strings.Contains(off, "Hostnames: off") {
		t.Errorf("disabled DNS attributes should read as off\n%s", off)
	}

	o := fullVPCOverview()
	o.Errs[aws.SectionDNS] = errors.New("AccessDenied")
	failed := plainVPC(overviewVPC(), o, stackedWidth)
	if !strings.Contains(failed, "DNS\nunavailable: AccessDenied") {
		t.Errorf("a failed DNS read should say so\n%s", failed)
	}
	if strings.Contains(failed, "Resolution:") {
		t.Errorf("a failed DNS read should not render an answer for either attribute\n%s", failed)
	}
}

// Each attachment is its own describe, so one denial costs its own section and leaves the rest of the pane standing.
func TestVPCOverviewSectionsFailIndependently(t *testing.T) {
	tests := []struct {
		section string
		want    string
	}{
		{aws.SectionDNS, "DNS\nunavailable: boom"},
		{aws.SectionSubnets, "Subnets\nunavailable: boom"},
		{aws.SectionIGW, "Internet gateway: unavailable: boom"},
		{aws.SectionNAT, "NAT gateways: unavailable: boom"},
		{aws.SectionEndpoints, "Endpoints\nunavailable: boom"},
	}

	for _, test := range tests {
		t.Run(test.section, func(t *testing.T) {
			o := fullVPCOverview()
			o.Errs[test.section] = errors.New("boom")
			got := plainVPC(overviewVPC(), o, stackedWidth)

			if !strings.Contains(got, test.want) {
				t.Errorf("overview is missing %q\n%s", test.want, got)
			}
			// The pane survives: the VPC is still identified and its own fields are still rendered.
			if !strings.Contains(got, "vpc-0abcdef1234567890") || !strings.Contains(got, "Configuration") {
				t.Errorf("a failed %s took the pane down with it\n%s", test.section, got)
			}
			if count := strings.Count(got, "unavailable"); count != 1 {
				t.Errorf("a failed %s made %d sections unavailable, want 1\n%s", test.section, count, got)
			}
		})
	}
}

// A subnet is public because the route table governing it reaches an internet gateway, which is the fetch's finding; the count has to follow that flag rather than the auto-assign setting beside it.
func TestVPCOverviewCountsPublicSubnetsByRouting(t *testing.T) {
	o := emptyVPCOverview()
	o.Subnets = []aws.Subnet{
		{ID: "subnet-a", AZ: "eu-west-1a", Public: true, MapPublicIPOnLaunch: false},
		{ID: "subnet-b", AZ: "eu-west-1a", Public: false, MapPublicIPOnLaunch: true},
	}

	if got := plainVPC(overviewVPC(), o, stackedWidth); !strings.Contains(got, "2 subnets · 1 public / 1 private") {
		t.Errorf("overview does not count public subnets by their routing\n%s", got)
	}
}

// Go randomizes map iteration, so an unsorted count line reshuffles itself on every re-render of the same VPC.
func TestVPCOverviewOrdersItsCountLines(t *testing.T) {
	o := emptyVPCOverview()
	o.Subnets = []aws.Subnet{
		{ID: "s1", AZ: "eu-west-1c"},
		{ID: "s2", AZ: "eu-west-1a"},
		{ID: "s3", AZ: "eu-west-1b"},
	}
	o.Endpoints = []aws.VPCEndpoint{
		{ID: "vpce-1", Type: "Interface", ServiceName: "com.amazonaws.eu-west-1.sts"},
		{ID: "vpce-2", Type: "Interface", ServiceName: "com.amazonaws.eu-west-1.ecr.api"},
	}

	got := plainVPC(overviewVPC(), o, stackedWidth)
	for _, want := range []string{
		"AZs: eu-west-1a (1), eu-west-1b (1), eu-west-1c (1)",
		"ecr.api, sts",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview is missing %q\n%s", want, got)
		}
	}
}

// Wrapping is off on an overview, so a line over its budget runs off the pane rather than folding.
func TestVPCOverviewNeverExceedsTheWidth(t *testing.T) {
	forceColor(t)

	vpc := overviewVPC()
	// A long name in the header, which Columns never measures because it spans the full width, and a tag value that runs past any column.
	vpc.Name = "a-very-long-vpc-name-nobody-should-have-but-someone-in-eu-west-1-does"
	vpc.Tags = append(vpc.Tags, aws.Tag{Key: "Description", Value: "the shared services network that everything in this account routes through, eventually"})

	for width := 40; width <= 220; width++ {
		for _, line := range strings.Split(FormatVPCOverview(vpc, fullVPCOverview(), width), "\n") {
			if got := runewidth.StringWidth(utils.Decolorise(line)); got > width {
				t.Fatalf("at width %d a line is %d cells wide: %q", width, got, utils.Decolorise(line))
			}
		}
	}
}
