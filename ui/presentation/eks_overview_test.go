package presentation

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// plainEKS renders the overview as the words it chose: escapes stripped and the alignment padding collapsed, since neither is what these states are about.
func plainEKS(c *aws.EKSCluster, o *aws.EKSOverview, width int) string {
	return kvPadding.ReplaceAllString(utils.Decolorise(FormatEKSClusterOverview(c, o, width)), " ")
}

func overviewEKSCluster() *aws.EKSCluster {
	return &aws.EKSCluster{
		Name:      "app-prod",
		Version:   "1.29",
		Status:    "ACTIVE",
		Endpoint:  "https://ABCD1234.gr7.eu-west-1.eks.amazonaws.com",
		Region:    "eu-west-1",
		CreatedAt: "2026-01-14 09:12:44",
		NodeCount: 6,
		Arn:       "arn:aws:eks:eu-west-1:123456789012:cluster/app-prod",
	}
}

// fullEKSOverview answers every fetch, so a test that removes one thing is testing that one thing.
// The node groups, the addons and the subnet ids are deliberately out of order: a sort is invisible to a fixture that is already sorted.
func fullEKSOverview() *aws.EKSOverview {
	return &aws.EKSOverview{
		Details: &aws.EKSClusterDetails{
			Name:                  "app-prod",
			PlatformVersion:       "eks.8",
			VpcId:                 "vpc-0abcdef1234567890",
			SubnetIds:             []string{"subnet-c", "subnet-a", "subnet-b"},
			SecurityGroupIds:      []string{"sg-0a1b2c3d"},
			EndpointPublicAccess:  true,
			EndpointPrivateAccess: true,
			PublicAccessCidrs:     []string{"10.0.0.0/8"},
			EnabledLogTypes:       []string{"api", "audit"},
			Tags:                  map[string]string{"Env": "prod", "Owner": "platform"},
		},
		NodeGroups: []aws.EKSNodeGroup{
			{Name: "workers-spot", Status: "ACTIVE", InstanceTypes: []string{"t3.medium"}, DesiredSize: 2, MinSize: 1, MaxSize: 4, Version: "1.29"},
			{Name: "general", Status: "ACTIVE", InstanceTypes: []string{"m6i.large"}, DesiredSize: 4, MinSize: 2, MaxSize: 8, Version: "1.29"},
		},
		Addons: []aws.EKSAddon{
			{Name: "vpc-cni", Version: "v1.18.1-eksbuild.1", Status: "ACTIVE", Health: "Healthy"},
			{Name: "coredns", Version: "v1.11.1-eksbuild.4", Status: "ACTIVE", Health: "Healthy"},
		},
		Errs: map[string]error{},
	}
}

// emptyEKSOverview is a cluster every describe answered for with nothing in it: no managed node groups, no addons, no tags, and control-plane logging off.
func emptyEKSOverview() *aws.EKSOverview {
	return &aws.EKSOverview{Details: &aws.EKSClusterDetails{}, Errs: map[string]error{}}
}

func TestEKSOverviewRendersEverySection(t *testing.T) {
	got := plainEKS(overviewEKSCluster(), fullEKSOverview(), stackedWidth)

	for _, want := range []string{
		"EKS cluster", "app-prod", "v1.29 · eu-west-1 · created 2026-01-14 09:12:44",
		"Configuration", "Version: 1.29", "Status: ACTIVE", "Endpoint: https://ABCD1234.gr7.eu-west-1.eks.amazonaws.com",
		"Region: eu-west-1", "Nodes: 6 desired", "Platform: eks.8", "ARN: arn:aws:eks:eu-west-1:123456789012:cluster/app-prod",
		"Networking", "VPC: vpc-0abcdef1234567890", "Endpoint access: public + private", "Allowed CIDRs: 10.0.0.0/8",
		"Control plane logging",
		"Tags", "Env: prod", "Owner: platform",
		"Node groups", "2 node groups · 6 nodes desired", "types: m6i.large (1), t3.medium (1)",
		"Addons", "2 addons",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview is missing %q\n%s", want, got)
		}
	}
}

// A cluster with nothing under it says so per section, rather than leaving four headings with nothing beneath them.
func TestEKSOverviewStatesEveryAbsence(t *testing.T) {
	got := plainEKS(&aws.EKSCluster{Name: "bare", Status: "CREATING"}, emptyEKSOverview(), stackedWidth)

	for _, want := range []string{
		"Version: none",
		"Endpoint: none",
		"Nodes: 0 desired",
		"Platform: none",
		"Subnets: none",
		"Security groups: none",
		// No managed node groups is not a broken cluster: Fargate and self-managed nodes are both normal, and the pane must not imply otherwise.
		"Node groups\nnone (Fargate or self-managed nodes)",
		"Addons\nnone",
		"Tags\nnone",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview is missing %q\n%s", want, got)
		}
	}
}

// The Configuration block reads the list row, not the describe, so the three fields the ticket names survive a denied DescribeCluster.
func TestEKSOverviewKeepsRowFieldsWhenTheDescribeFails(t *testing.T) {
	o := fullEKSOverview()
	o.Details = nil
	o.Errs[aws.SectionCluster] = errors.New("AccessDeniedException")

	got := plainEKS(overviewEKSCluster(), o, stackedWidth)

	for _, want := range []string{"Version: 1.29", "Status: ACTIVE", "Endpoint: https://ABCD1234.gr7.eu-west-1.eks.amazonaws.com"} {
		if !strings.Contains(got, want) {
			t.Errorf("a failed describe lost %q, which comes off the list row\n%s", want, got)
		}
	}
	// The describe-only field is the one that has to report the failure, in its own place rather than by disappearing.
	if !strings.Contains(got, "Platform: unavailable: AccessDeniedException") {
		t.Errorf("overview does not report the failed describe on its own row\n%s", got)
	}
	// The two lists are separate fetches and answered, so they must still render.
	if !strings.Contains(got, "2 node groups") || !strings.Contains(got, "2 addons") {
		t.Errorf("a failed describe took the node groups or addons down with it\n%s", got)
	}
}

// A describe that never answered is not a describe that failed, and neither may be a nil dereference: the sections reading it are the only thing standing between a nil Details and a panic that takes the app down rather than the tab.
func TestEKSOverviewSeparatesADescribeThatFailedFromOneThatNeverAnswered(t *testing.T) {
	o := fullEKSOverview()
	o.Details = nil

	got := plainEKS(overviewEKSCluster(), o, stackedWidth)

	for _, want := range []string{
		"Networking\nunavailable: cluster not described",
		"Control plane logging\nunavailable: cluster not described",
		"Tags\nunavailable: cluster not described",
		"Platform: unavailable: cluster not described",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview is missing %q\n%s", want, got)
		}
	}
}

// Each of the three fetches is its own call, so one denial costs its own sections and leaves the rest of the pane standing.
func TestEKSOverviewSectionsFailIndependently(t *testing.T) {
	tests := []struct {
		section string
		want    []string
		intact  []string
	}{
		{
			section: aws.SectionCluster,
			want:    []string{"Networking\nunavailable: boom", "Control plane logging\nunavailable: boom", "Tags\nunavailable: boom"},
			intact:  []string{"2 node groups", "2 addons"},
		},
		{
			section: aws.SectionNodeGroups,
			want:    []string{"Node groups\nunavailable: boom"},
			intact:  []string{"Networking", "2 addons", "Tags"},
		},
		{
			section: aws.SectionAddons,
			want:    []string{"Addons\nunavailable: boom"},
			intact:  []string{"Networking", "2 node groups", "Tags"},
		},
	}

	for _, test := range tests {
		t.Run(test.section, func(t *testing.T) {
			o := fullEKSOverview()
			o.Errs[test.section] = errors.New("boom")
			got := plainEKS(overviewEKSCluster(), o, stackedWidth)

			for _, want := range test.want {
				if !strings.Contains(got, want) {
					t.Errorf("overview is missing %q\n%s", want, got)
				}
			}
			for _, want := range test.intact {
				if !strings.Contains(got, want) {
					t.Errorf("a failed %s took %q down with it\n%s", test.section, want, got)
				}
			}
			// The pane survives: the cluster is still identified and the row's own fields are still rendered.
			if !strings.Contains(got, "app-prod") || !strings.Contains(got, "Version: 1.29") {
				t.Errorf("a failed %s took the pane down with it\n%s", test.section, got)
			}
		})
	}
}

// A node group trailing the control plane is the drift that makes a cluster upgrade fail halfway, so it is marked rather than left to a version-against-version reading.
func TestEKSOverviewMarksNodeGroupVersionDrift(t *testing.T) {
	o := fullEKSOverview()
	o.NodeGroups = []aws.EKSNodeGroup{
		{Name: "behind", Status: "ACTIVE", DesiredSize: 1, MinSize: 1, MaxSize: 1, Version: "1.27"},
		{Name: "current", Status: "ACTIVE", DesiredSize: 1, MinSize: 1, MaxSize: 1, Version: "1.29"},
		{Name: "unreported", Status: "ACTIVE", DesiredSize: 1, MinSize: 1, MaxSize: 1},
	}

	got := plainEKS(overviewEKSCluster(), o, stackedWidth)

	if !strings.Contains(got, "v1.27 ⚠") {
		t.Errorf("a node group behind the control plane is not marked\n%s", got)
	}
	if strings.Contains(got, "v1.29 ⚠") {
		t.Errorf("a node group on the cluster's own version is marked as drifted\n%s", got)
	}
	// An unknown version is not evidence of a mismatch, so it must not be reported as one.
	if strings.Count(got, "⚠") != 1 {
		t.Errorf("overview marks %d node groups as drifted, want 1\n%s", strings.Count(got, "⚠"), got)
	}
}

// The colour carries the same finding as the marker, and only the escape can prove it: a drifted version has to be the amber one.
func TestEKSNodeGroupVersionCellColoursOnlyTheDrift(t *testing.T) {
	drifted := eksNodeGroupVersionCell(aws.EKSNodeGroup{Version: "1.27"}, "1.29")
	if drifted.Color == 0 {
		t.Errorf("eksNodeGroupVersionCell(1.27, 1.29) = %+v, want it coloured", drifted)
	}

	current := eksNodeGroupVersionCell(aws.EKSNodeGroup{Version: "1.29"}, "1.29")
	if current.Color != 0 {
		t.Errorf("eksNodeGroupVersionCell(1.29, 1.29) = %+v, want it unstyled", current)
	}
	if current.Text != "v1.29" {
		t.Errorf("eksNodeGroupVersionCell(1.29, 1.29).Text = %q, want %q", current.Text, "v1.29")
	}

	// A cluster whose own version is unknown cannot establish drift for anything.
	unknownCluster := eksNodeGroupVersionCell(aws.EKSNodeGroup{Version: "1.27"}, "")
	if unknownCluster.Color != 0 {
		t.Errorf("eksNodeGroupVersionCell(1.27, \"\") = %+v, want it unstyled: no cluster version means no drift to report", unknownCluster)
	}
}

// The describe omits an addon's health block entirely on some addons, and reading that as healthy hides the one state worth acting on.
func TestEKSAddonHealthSeparatesUnreportedFromHealthy(t *testing.T) {
	tests := []struct {
		health string
		want   string
	}{
		{health: "", want: "-"},
		{health: "Healthy", want: "healthy"},
		{health: "AccessDenied", want: "AccessDenied"},
	}

	for _, test := range tests {
		if got := eksAddonHealthCell(test.health).Text; got != test.want {
			t.Errorf("eksAddonHealthCell(%q).Text = %q, want %q", test.health, got, test.want)
		}
	}

	// An unreported health must not be coloured as an answer, and a real issue must not be left plain.
	if got := eksAddonHealthCell(""); got.Text == "healthy" {
		t.Errorf("eksAddonHealthCell(\"\") = %+v, want it distinct from a healthy addon", got)
	}
	if got := eksAddonHealthCell("AccessDenied"); got.Color == 0 {
		t.Errorf("eksAddonHealthCell(%q) = %+v, want an unhealthy addon coloured", "AccessDenied", got)
	}
}

// A group already at its maximum cannot absorb a scale-up, which is the whole reason the bounds are rendered beside the desired count rather than the count alone.
// Asserted as the whole label: three numbers in one cell are the easiest place for a swap to hide, and the render tests read the rows through a space-collapsing filter that cannot see which bound is which.
func TestEKSScalingLabelReportsDesiredWithinItsBounds(t *testing.T) {
	tests := []struct {
		group aws.EKSNodeGroup
		want  string
	}{
		{group: aws.EKSNodeGroup{DesiredSize: 4, MinSize: 2, MaxSize: 8}, want: "4 (2-8)"},
		// At the ceiling, which is the state the bounds exist to make visible.
		{group: aws.EKSNodeGroup{DesiredSize: 6, MinSize: 1, MaxSize: 6}, want: "6 (1-6)"},
		// A scaled-to-zero group is a real state and must not render as absent.
		{group: aws.EKSNodeGroup{DesiredSize: 0, MinSize: 0, MaxSize: 10}, want: "0 (0-10)"},
	}

	for _, test := range tests {
		if got := eksScalingLabel(test.group); got != test.want {
			t.Errorf("eksScalingLabel(desired %d, min %d, max %d) = %q, want %q", test.group.DesiredSize, test.group.MinSize, test.group.MaxSize, got, test.want)
		}
	}
}

// Each table's row is asserted whole, because a per-word Contains cannot see a column that swapped places with its neighbour or a cell that went missing.
func TestEKSOverviewRendersItsTableRowsInFull(t *testing.T) {
	got := plainEKS(overviewEKSCluster(), fullEKSOverview(), stackedWidth)

	for _, want := range []string{
		"general ▶ ACTIVE 4 (2-8) v1.29",
		"workers-spot ▶ ACTIVE 2 (1-4) v1.29",
		"coredns ▶ ACTIVE v1.11.1-eksbuild.4 healthy",
		"vpc-cni ▶ ACTIVE v1.18.1-eksbuild.1 healthy",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview is missing the row %q\n%s", want, got)
		}
	}
}

// Neither list arrives ordered, so an unsorted table reshuffles itself between renders of the same cluster.
func TestEKSOverviewOrdersItsTablesByName(t *testing.T) {
	got := plainEKS(overviewEKSCluster(), fullEKSOverview(), stackedWidth)

	if general, spot := strings.Index(got, "general"), strings.Index(got, "workers-spot"); general > spot {
		t.Errorf("node groups render out of name order (general at %d, workers-spot at %d)\n%s", general, spot, got)
	}
	if coredns, cni := strings.Index(got, "coredns"), strings.Index(got, "vpc-cni"); coredns > cni {
		t.Errorf("addons render out of name order (coredns at %d, vpc-cni at %d)\n%s", coredns, cni, got)
	}
}

// Go randomizes map iteration, so an unsorted tag block reshuffles itself on every re-render of the same cluster.
// Six keys, because with three a dropped sort still lands in order once in six runs.
func TestEKSOverviewSortsItsTagsAndIDs(t *testing.T) {
	o := fullEKSOverview()
	o.Details.Tags = map[string]string{"f": "6", "c": "3", "a": "1", "e": "5", "b": "2", "d": "4"}

	got := plainEKS(overviewEKSCluster(), o, stackedWidth)

	if want := "Tags\na: 1\nb: 2\nc: 3\nd: 4\ne: 5\nf: 6"; !strings.Contains(got, want) {
		t.Errorf("tag block is not in key order, want %q\n%s", want, got)
	}
	// Asserted as the whole line: a Contains on the prefix cannot see an id the line grew or lost.
	if want := "Subnets: 3 · subnet-a, subnet-b, subnet-c\n"; !strings.Contains(got, want) {
		t.Errorf("subnet ids are not counted and sorted, want %q\n%s", want, got)
	}
}

// Whether the control plane is reachable from the internet is the posture question asked of an EKS cluster, and it is four answers rather than two.
func TestEKSOverviewReportsEndpointAccessPosture(t *testing.T) {
	tests := []struct {
		name         string
		public       bool
		private      bool
		wantAccess   string
		wantColoured bool
	}{
		{name: "both", public: true, private: true, wantAccess: "public + private", wantColoured: true},
		{name: "public only", public: true, private: false, wantAccess: "public only", wantColoured: true},
		{name: "private only", public: false, private: true, wantAccess: "private only", wantColoured: false},
		// EKS requires at least one, so neither flag set is reported as read rather than folded into a posture the cluster does not have.
		{name: "neither", public: false, private: false, wantAccess: "none reported", wantColoured: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			forceColor(t)

			d := &aws.EKSClusterDetails{EndpointPublicAccess: test.public, EndpointPrivateAccess: test.private}
			got := eksEndpointAccess(d)

			if utils.Decolorise(got) != test.wantAccess {
				t.Errorf("eksEndpointAccess() = %q, want %q", utils.Decolorise(got), test.wantAccess)
			}
			if coloured := got != utils.Decolorise(got); coloured != test.wantColoured {
				t.Errorf("eksEndpointAccess() coloured = %v, want %v: only an internet-reachable control plane is called out", coloured, test.wantColoured)
			}
		})
	}
}

// The CIDR list restricts the public endpoint and nothing else, so beside a disabled endpoint it would read as an exposure that is switched off.
func TestEKSOverviewHidesPublicCIDRsWhenPublicAccessIsOff(t *testing.T) {
	forceColor(t)

	off := &aws.EKSClusterDetails{EndpointPrivateAccess: true, PublicAccessCidrs: []string{"0.0.0.0/0"}}
	if got := eksPublicAccessCIDRs(off); got != "n/a, public access off" {
		t.Errorf("eksPublicAccessCIDRs() = %q, want it withheld while the public endpoint is off", got)
	}

	// Open to the whole internet is also the AWS default, so it is reached by not deciding and has to be called out.
	open := &aws.EKSClusterDetails{EndpointPublicAccess: true, PublicAccessCidrs: []string{"0.0.0.0/0"}}
	gotOpen := eksPublicAccessCIDRs(open)
	if utils.Decolorise(gotOpen) != "0.0.0.0/0" {
		t.Errorf("eksPublicAccessCIDRs() = %q, want the CIDR listed", utils.Decolorise(gotOpen))
	}
	if gotOpen == utils.Decolorise(gotOpen) {
		t.Errorf("eksPublicAccessCIDRs() = %q, want 0.0.0.0/0 coloured", gotOpen)
	}

	// A restricted list is the deliberate state and must not be coloured as a finding.
	restricted := &aws.EKSClusterDetails{EndpointPublicAccess: true, PublicAccessCidrs: []string{"10.0.0.0/8"}}
	if got := eksPublicAccessCIDRs(restricted); got != "10.0.0.0/8" {
		t.Errorf("eksPublicAccessCIDRs() = %q, want an unstyled %q", got, "10.0.0.0/8")
	}
}

// An audit trail nobody switched on is invisible exactly when it is needed, and the absence of a line does not say that: every type EKS can emit is reported either way.
func TestEKSOverviewNamesEveryDisabledLogType(t *testing.T) {
	forceColor(t)

	partial := plainEKS(overviewEKSCluster(), fullEKSOverview(), stackedWidth)
	// Whole lines: a Contains on the prefix cannot see a type the line grew or lost.
	if want := "Enabled: api, audit\n"; !strings.Contains(partial, want) {
		t.Errorf("logging block is missing %q\n%s", want, partial)
	}
	if want := "Disabled: authenticator, controllerManager, scheduler\n"; !strings.Contains(partial, want) {
		t.Errorf("logging block is missing %q\n%s", want, partial)
	}

	o := fullEKSOverview()
	o.Details.EnabledLogTypes = eksLogTypes
	full := plainEKS(overviewEKSCluster(), o, stackedWidth)
	if want := "Disabled: none\n"; !strings.Contains(full, want) {
		t.Errorf("a fully-logged cluster should report nothing disabled, want %q\n%s", want, full)
	}

	// A partial configuration reads as configured until the missing types are named, so it is the one that carries the colour.
	if got := eksDisabledLogTypes([]string{"audit"}); got == utils.Decolorise(got) {
		t.Errorf("eksDisabledLogTypes(%v) = %q, want it coloured", []string{"audit"}, got)
	}
	if got := eksDisabledLogTypes(nil); got != "none" {
		t.Errorf("eksDisabledLogTypes(nil) = %q, want an unstyled %q", got, "none")
	}
}

// Wrapping is off on an overview, so a line over its budget runs off the pane rather than folding.
func TestEKSOverviewNeverExceedsTheWidth(t *testing.T) {
	forceColor(t)

	cluster := overviewEKSCluster()
	// A long name in the header, which Columns never measures because it spans the full width.
	cluster.Name = "a-very-long-eks-cluster-name-nobody-should-have-but-someone-in-eu-west-1-does"

	o := fullEKSOverview()
	o.Details.Tags["Description"] = "the shared platform cluster that every service in this account eventually schedules onto"
	o.NodeGroups = append(o.NodeGroups, aws.EKSNodeGroup{
		Name:        "a-node-group-with-a-name-long-enough-to-run-past-any-column-it-is-given",
		Status:      "DEGRADED",
		DesiredSize: 12, MinSize: 3, MaxSize: 40,
		Version: "1.27",
	})

	for width := 40; width <= 220; width++ {
		for _, line := range strings.Split(FormatEKSClusterOverview(cluster, o, width), "\n") {
			if got := runewidth.StringWidth(utils.Decolorise(line)); got > width {
				t.Fatalf("at width %d a line is %d cells wide: %q", width, got, utils.Decolorise(line))
			}
		}
	}
}
