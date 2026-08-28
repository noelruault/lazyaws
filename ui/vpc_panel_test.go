package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/noelruault/lazyaws/apps/aws"
)

func TestFormatVPCSubnetsSeparatesReachFromAutoAssign(t *testing.T) {
	out := formatVPCSubnets([]aws.Subnet{
		{ID: "subnet-pub", CIDR: "10.0.1.0/24", AZ: "eu-west-1a", State: "available", Public: true, MapPublicIPOnLaunch: true, AvailableIPs: 250},
		{ID: "subnet-priv", CIDR: "10.0.2.0/24", AZ: "eu-west-1b", State: "available", AvailableIPs: 12},
		// A subnet can route to an internet gateway while still not handing out public IPs; these must not collapse into one flag.
		{ID: "subnet-mixed", CIDR: "10.0.3.0/24", AZ: "eu-west-1c", State: "available", Public: true, AvailableIPs: 4},
	})

	for _, want := range []string{"subnet-pub", "public +autoip", "subnet-priv", "private", "250 free"} {
		if !strings.Contains(out, want) {
			t.Errorf("subnets missing %q:\n%s", want, out)
		}
	}

	if strings.Count(out, "+autoip") != 1 {
		t.Errorf("only the auto-assigning subnet may be marked +autoip:\n%s", out)
	}
}

func TestFormatVPCSubnetsEmpty(t *testing.T) {
	if out := formatVPCSubnets(nil); !strings.Contains(out, "no subnets") {
		t.Errorf("empty subnet list = %q", out)
	}
}

func TestFormatVPCRouteTablesMarksMainAndAssociations(t *testing.T) {
	out := formatVPCRouteTables([]aws.RouteTable{
		{
			ID:     "rtb-main",
			Main:   true,
			Routes: []aws.Route{{Destination: "0.0.0.0/0", Target: "igw-1", State: "active", Origin: "CreateRoute"}},
		},
		{
			ID:        "rtb-private",
			Name:      "private",
			SubnetIDs: []string{"subnet-a", "subnet-b"},
			Routes:    []aws.Route{{Destination: "10.1.0.0/16", Target: "pcx-0fedcba987654321", State: "active", Origin: "CreateRoute"}},
		},
	})

	for _, want := range []string{"rtb-main", "(main)", "igw-1", "private", "subnet-a, subnet-b", "pcx-0fedcba987654321"} {
		if !strings.Contains(out, want) {
			t.Errorf("route tables missing %q:\n%s", want, out)
		}
	}
}

func TestFormatVPCRouteTablesHandlesATableWithNoRoutes(t *testing.T) {
	if out := formatVPCRouteTables([]aws.RouteTable{{ID: "rtb-empty"}}); !strings.Contains(out, "no routes") {
		t.Errorf("a table with no routes = %q", out)
	}
}

func TestFormatVPCGateways(t *testing.T) {
	out := formatVPCGateways(
		[]aws.InternetGateway{{ID: "igw-1", Name: "main-igw", State: "available"}},
		[]aws.NATGateway{{ID: "nat-1", State: "available", ConnectivityType: "public", SubnetID: "subnet-a", PublicIP: "52.1.2.3", PrivateIP: "10.0.1.5"}},
	)

	for _, want := range []string{"igw-1", "main-igw", "nat-1", "52.1.2.3", "10.0.1.5"} {
		if !strings.Contains(out, want) {
			t.Errorf("gateways missing %q:\n%s", want, out)
		}
	}
}

// A VPC with no gateway of either kind is exactly the state worth seeing, so neither section may be silently dropped.
func TestFormatVPCGatewaysShowsBothSectionsWhenEmpty(t *testing.T) {
	out := formatVPCGateways(nil, nil)

	if !strings.Contains(out, "Internet gateways:") || !strings.Contains(out, "NAT gateways:") {
		t.Errorf("both headings must survive an empty result:\n%s", out)
	}
	if strings.Count(out, "none") != 2 {
		t.Errorf("both sections must report none:\n%s", out)
	}
}

func TestFormatVPCGatewaysSurfacesNATFailure(t *testing.T) {
	out := formatVPCGateways(nil, []aws.NATGateway{
		{ID: "nat-bad", State: "failed", FailureMessage: "elastic ip already in use"},
	})

	if !strings.Contains(out, "elastic ip already in use") {
		t.Errorf("nat failure message not shown:\n%s", out)
	}
}

func TestVPCEndpointRowCells(t *testing.T) {
	iface := vpcEndpointRowCells(&aws.VPCEndpoint{
		ID: "vpce-1", ServiceName: "com.amazonaws.eu-west-1.secretsmanager",
		Type: "Interface", State: "Available", PrivateDNSEnabled: true,
	})

	if iface[1] != "secretsmanager" {
		t.Errorf("label cell = %q, want the shortened service", iface[1])
	}
	if iface[4] != "private-dns enabled" {
		t.Errorf("private dns cell = %q", iface[4])
	}

	// Private DNS is meaningless on a gateway endpoint, so it must not be reported as disabled there.
	gateway := vpcEndpointRowCells(&aws.VPCEndpoint{
		ID: "vpce-2", ServiceName: "com.amazonaws.eu-west-1.s3", Type: "Gateway", State: "Available",
	})
	if gateway[4] != "-" {
		t.Errorf("gateway endpoint reported a private-dns state: %q", gateway[4])
	}
}

// Enter exists to surface the fields the row has no width for; each one below is already in memory when the row is drawn.
func TestFormatVPCEndpointDetailShowsWhatTheRowCannot(t *testing.T) {
	out := formatVPCEndpointDetail(&aws.VPCEndpoint{
		ID:                  "vpce-1",
		ServiceName:         "com.amazonaws.eu-west-1.ecr.api",
		Type:                "Interface",
		State:               "Available",
		VpcID:               "vpc-1",
		SubnetIDs:           []string{"subnet-a", "subnet-b"},
		NetworkInterfaceIDs: []string{"eni-1"},
		SecurityGroups:      []aws.SecurityGroup{{ID: "sg-1", Name: "endpoint-sg"}},
		DNSNames:            []string{"vpce-1.api.ecr.eu-west-1.vpce.amazonaws.com"},
		PolicyDocument:      `{"Statement":[]}`,
		Tags:                []aws.Tag{{Key: "env", Value: "stage"}},
	})

	for _, want := range []string{
		"vpce-1",
		"Subnets:", "subnet-a",
		"Security groups:", "sg-1 (endpoint-sg)",
		"Network interfaces:", "eni-1",
		"DNS names:",
		"Tags:", "env=stage",
		"Policy:", "Statement",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("detail is missing %q:\n%s", want, out)
		}
	}
}

func TestFormatVPCEndpointDetailOmitsEmptySections(t *testing.T) {
	out := formatVPCEndpointDetail(&aws.VPCEndpoint{ID: "vpce-gw", Type: "Gateway", State: "Available"})

	for _, unwanted := range []string{"Subnets:", "Security groups:", "DNS names:", "Tags:", "Private DNS"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("empty section %q rendered anyway:\n%s", unwanted, out)
		}
	}
	// A gateway endpoint has no policy of its own more often than not, and "no policy" beats a blank heading.
	if !strings.Contains(out, "no policy") {
		t.Errorf("absent policy should say so:\n%s", out)
	}
}

func TestFormatVPCTransitReportsNoAttachment(t *testing.T) {
	out := formatVPCTransit("vpc-1", []aws.TGWAttachment{{ID: "tgw-attach-1", ResourceID: "vpc-other"}}, nil)

	if !strings.Contains(out, "no transit gateway attachment") {
		t.Errorf("a VPC with no attachment of its own = %q", out)
	}
}

func TestFormatVPCTransitJoinsTheGateway(t *testing.T) {
	created := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	out := formatVPCTransit("vpc-1",
		[]aws.TGWAttachment{{
			ID:               "tgw-attach-1",
			TransitGatewayID: "tgw-9",
			ResourceID:       "vpc-1",
			ResourceOwnerID:  "123456789012",
			TGWOwnerID:       "210987654321",
			State:            "available",
			RouteTableID:     "tgw-rtb-1",
			AssociationState: "associated",
			CreatedAt:        &created,
		}},
		[]aws.TransitGateway{{ID: "tgw-9", State: "available", AmazonSideASN: 64512, Description: "shared core"}},
	)

	for _, want := range []string{"tgw-attach-1", "tgw-9", "tgw-rtb-1", "64512", "shared core"} {
		if !strings.Contains(out, want) {
			t.Errorf("transit detail missing %q:\n%s", want, out)
		}
	}

	// A gateway owned by another account is the usual reason an attachment exists while traffic still does not flow.
	if !strings.Contains(out, "shared from another account") {
		t.Errorf("cross-account ownership not flagged:\n%s", out)
	}
}

// An attachment to a gateway this account cannot describe must still render, saying so rather than looking like a healthy one.
func TestFormatVPCTransitHandlesAnInvisibleGateway(t *testing.T) {
	out := formatVPCTransit("vpc-1",
		[]aws.TGWAttachment{{ID: "tgw-attach-1", TransitGatewayID: "tgw-elsewhere", ResourceID: "vpc-1", State: "available"}},
		nil,
	)

	if !strings.Contains(out, "not visible from this account") {
		t.Errorf("unknown gateway not reported:\n%s", out)
	}
}

func TestVPCEndpointConsoleURL(t *testing.T) {
	got := vpcEndpointConsoleURL("eu-west-1", "vpce-abc")
	for _, want := range []string{"eu-west-1.console.aws.amazon.com", "region=eu-west-1", "vpcEndpointId=vpce-abc"} {
		if !strings.Contains(got, want) {
			t.Errorf("console url %q is missing %q", got, want)
		}
	}

	// Before the first successful call the client has no region, and a URL naming region "" would 404 rather than land on the console.
	if got := vpcEndpointConsoleURL("", "vpce-abc"); strings.Contains(got, "region=") {
		t.Errorf("regionless url = %q, want the plain console entry point", got)
	}
}
