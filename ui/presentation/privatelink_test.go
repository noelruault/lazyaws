package presentation

import (
	"errors"
	"strings"
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// A column of zeroes would bury the one row that needs attention, which is the only thing this column is for.
func TestPendingCellIsBlankUntilSomethingIsWaiting(t *testing.T) {
	if got := pendingCell(0); got.Text != "" {
		t.Errorf("pendingCell(0) = %q, want nothing", got.Text)
	}
	if got := pendingCell(3); !strings.Contains(got.Text, "3") || got.Color == 0 {
		t.Errorf("pendingCell(3) = %+v, want a coloured count", got)
	}
}

func TestEndpointServiceRowLeadsWithTheNameAndTheWait(t *testing.T) {
	cells := GetVPCEndpointServiceDisplayCells(&aws.VPCEndpointService{
		ID:      "vpce-svc-0123456789abcdef0",
		NameTag: "payments",
		State:   "Available",
		Pending: 2,
	})
	if len(cells) != len(PrivateLinkWeights()) {
		t.Fatalf("%d cells against %d weights: RenderTableFit refuses the row", len(cells), len(PrivateLinkWeights()))
	}

	row := utils.Decolorise(joinCells(cells))
	for _, want := range []string{"payments", "2 pending"} {
		if !strings.Contains(row, want) {
			t.Errorf("row %q omits %q", row, want)
		}
	}
}

func joinCells(cells []utils.Cell) string {
	parts := make([]string, len(cells))
	for i, cell := range cells {
		parts[i] = cell.Text
	}
	return strings.Join(parts, " ")
}

func TestEndpointServiceOverviewReportsTheWaitAndTheStates(t *testing.T) {
	service := &aws.VPCEndpointService{
		ID:                 "vpce-svc-0123456789abcdef0",
		Name:               "com.amazonaws.vpce.eu-west-1.vpce-svc-0123456789abcdef0",
		NameTag:            "payments",
		State:              "Available",
		AcceptanceRequired: true,
		Pending:            1,
		AvailabilityZones:  []string{"eu-west-1a"},
		PrivateDNSName:     "api.internal.example",
		LoadBalancerARNs:   []string{"arn:aws:elasticloadbalancing:eu-west-1:111122223333:loadbalancer/net/api-nlb/abc123"},
		Tags:               []aws.Tag{{Key: "team", Value: "platform"}},
	}
	connections := []aws.VPCEndpointConnection{
		{EndpointID: "vpce-1", State: "available"},
		{EndpointID: "vpce-2", State: "available"},
		{EndpointID: "vpce-3", State: aws.VPCEndpointStatePendingAcceptance},
	}

	out := utils.Decolorise(FormatVPCEndpointServiceOverview(service, connections, nil, 120))
	for _, want := range []string{"payments", "vpce-svc-0123456789abcdef0", "acceptance required", "api.internal.example", "api-nlb", "team", "platform"} {
		if !strings.Contains(out, want) {
			t.Errorf("overview omits %q:\n%s", want, out)
		}
	}
	// The connections block is the count by state, so both states have to appear with their own number.
	if !strings.Contains(out, "available") || !strings.Contains(out, aws.VPCEndpointStatePendingAcceptance) {
		t.Errorf("overview does not break the connections down by state:\n%s", out)
	}
}

// A connections read that failed and a service with no connections look identical once the count is gone, and only one of them means nobody is waiting.
func TestEndpointServiceOverviewSaysWhenTheConnectionsCouldNotBeRead(t *testing.T) {
	service := &aws.VPCEndpointService{ID: "vpce-svc-1", State: "Available", AcceptanceRequired: true}

	out := utils.Decolorise(FormatVPCEndpointServiceOverview(service, nil, errors.New("AccessDenied"), 120))
	if !strings.Contains(out, "AccessDenied") || !strings.Contains(out, "unavailable") {
		t.Errorf("overview hides the failed connections read:\n%s", out)
	}

	none := utils.Decolorise(FormatVPCEndpointServiceOverview(service, nil, nil, 120))
	if strings.Contains(none, "unavailable") {
		t.Errorf("a service with no connections reads as unavailable:\n%s", none)
	}
}

func TestLoadBalancerNameDropsTheARNAroundIt(t *testing.T) {
	cases := map[string]string{
		"arn:aws:elasticloadbalancing:eu-west-1:111122223333:loadbalancer/net/api-nlb/abc123": "api-nlb",
		"arn:aws:elasticloadbalancing:eu-west-1:111122223333:loadbalancer/gwy/edge/def456":    "edge",
		"not-an-arn": "not-an-arn",
	}
	for arn, want := range cases {
		if got := loadBalancerName(arn); got != want {
			t.Errorf("loadBalancerName(%q) = %q, want %q", arn, got, want)
		}
	}
}
