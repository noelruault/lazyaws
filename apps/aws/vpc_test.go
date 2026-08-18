package aws

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestShortService(t *testing.T) {
	cases := []struct {
		name    string
		service string
		want    string
	}{
		{"gateway service", "com.amazonaws.eu-west-1.s3", "s3"},
		{"hyphenated service", "com.amazonaws.eu-west-1.execute-api", "execute-api"},
		{"service with a dotted suffix", "com.amazonaws.eu-west-1.sagemaker.api", "sagemaker.api"},
		{"privatelink service keeps its id", "com.amazonaws.vpce.eu-west-1.vpce-svc-0a1b2c3d", "vpce-svc-0a1b2c3d"},
		{"third-party service is left alone", "com.example.eu-west-1.my-service", "com.example.eu-west-1.my-service"},
		{"truncated name is left alone", "com.amazonaws.eu-west-1", "com.amazonaws.eu-west-1"},
		{"empty stays empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := (VPCEndpoint{ServiceName: tc.service}).ShortService(); got != tc.want {
				t.Errorf("ShortService(%q) = %q, want %q", tc.service, got, tc.want)
			}
		})
	}
}

func TestNewVPCEndpointReadsEveryField(t *testing.T) {
	created := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	endpoint := newVPCEndpoint(types.VpcEndpoint{
		VpcEndpointId:       aws.String("vpce-0123456789abcdef0"),
		ServiceName:         aws.String("com.amazonaws.eu-west-1.secretsmanager"),
		VpcEndpointType:     types.VpcEndpointTypeInterface,
		State:               types.StateAvailable,
		VpcId:               aws.String("vpc-0123456789abcdef0"),
		OwnerId:             aws.String("123456789012"),
		IpAddressType:       types.IpAddressTypeIpv4,
		PrivateDnsEnabled:   aws.Bool(true),
		RequesterManaged:    aws.Bool(false),
		CreationTimestamp:   &created,
		SubnetIds:           []string{"subnet-a", "subnet-b"},
		NetworkInterfaceIds: []string{"eni-1"},
		PolicyDocument:      aws.String(`{"Statement":[]}`),
		Groups: []types.SecurityGroupIdentifier{
			{GroupId: aws.String("sg-1"), GroupName: aws.String("endpoint-sg")},
		},
		DnsEntries: []types.DnsEntry{
			{DnsName: aws.String("vpce-0123.secretsmanager.eu-west-1.vpce.amazonaws.com")},
			{DnsName: nil},
			{DnsName: aws.String("")},
		},
		Tags: []types.Tag{
			{Key: aws.String("Name"), Value: aws.String("secrets-endpoint")},
			{Key: aws.String("env"), Value: aws.String("stage")},
		},
	})

	if endpoint.ID != "vpce-0123456789abcdef0" {
		t.Errorf("ID = %q", endpoint.ID)
	}
	if endpoint.Type != "Interface" {
		t.Errorf("Type = %q, want Interface", endpoint.Type)
	}
	// Endpoint states are TitleCase, unlike the lowercase instance states elsewhere in EC2; the presentation layer matches on these exact spellings.
	if endpoint.State != "Available" {
		t.Errorf("State = %q, want Available", endpoint.State)
	}
	if endpoint.Name != "secrets-endpoint" {
		t.Errorf("Name = %q, want the Name tag", endpoint.Name)
	}
	if !endpoint.PrivateDNSEnabled {
		t.Error("PrivateDNSEnabled = false, want true")
	}
	if endpoint.RequesterManaged {
		t.Error("RequesterManaged = true, want false")
	}
	if endpoint.CreatedAt == nil || !endpoint.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want %v", endpoint.CreatedAt, created)
	}
	if len(endpoint.SecurityGroups) != 1 || endpoint.SecurityGroups[0].Name != "endpoint-sg" {
		t.Errorf("SecurityGroups = %+v", endpoint.SecurityGroups)
	}
	if len(endpoint.Tags) != 2 {
		t.Errorf("Tags = %+v, want both preserved", endpoint.Tags)
	}
	// A nil or empty DnsName would render as a blank line in the detail view, so both are dropped.
	if len(endpoint.DNSNames) != 1 {
		t.Errorf("DNSNames = %q, want only the populated entry", endpoint.DNSNames)
	}
}

func TestNewVPCEndpointSurvivesAnEmptyRecord(t *testing.T) {
	endpoint := newVPCEndpoint(types.VpcEndpoint{})

	if endpoint.ID != "" || endpoint.Name != "" || endpoint.PolicyDocument != "" {
		t.Errorf("empty record produced %+v", endpoint)
	}
	if endpoint.PrivateDNSEnabled || endpoint.RequesterManaged {
		t.Error("nil bools must read as false")
	}
	if endpoint.CreatedAt != nil {
		t.Error("nil timestamp must stay nil")
	}
	if endpoint.LastError != "" {
		t.Errorf("LastError = %q, want empty when the SDK sends none", endpoint.LastError)
	}
}

func TestNewVPCEndpointJoinsLastError(t *testing.T) {
	both := newVPCEndpoint(types.VpcEndpoint{
		LastError: &types.LastError{Code: aws.String("InsufficientCapacity"), Message: aws.String("no capacity in az")},
	})
	if both.LastError != "InsufficientCapacity no capacity in az" {
		t.Errorf("LastError = %q", both.LastError)
	}

	// A code with no message must not leave a trailing space, which would look like corrupt output in the panel.
	codeOnly := newVPCEndpoint(types.VpcEndpoint{
		LastError: &types.LastError{Code: aws.String("InsufficientCapacity")},
	})
	if codeOnly.LastError != "InsufficientCapacity" {
		t.Errorf("LastError = %q, want no padding", codeOnly.LastError)
	}
}

func TestPublicSubnetsUsesTheGoverningRouteTable(t *testing.T) {
	tables := []RouteTable{
		{
			ID:     "rtb-main",
			VpcID:  "vpc-1",
			Main:   true,
			Routes: []Route{{Destination: "0.0.0.0/0", Target: "igw-1"}},
		},
		{
			ID:        "rtb-private",
			VpcID:     "vpc-1",
			SubnetIDs: []string{"subnet-private"},
			Routes:    []Route{{Destination: "0.0.0.0/0", Target: "nat-1"}},
		},
		{
			ID:        "rtb-public",
			VpcID:     "vpc-1",
			SubnetIDs: []string{"subnet-public"},
			Routes:    []Route{{Destination: "0.0.0.0/0", Target: "igw-1"}},
		},
	}
	reach := publicSubnets(tables)

	cases := []struct {
		name     string
		subnetID string
		want     bool
	}{
		{"explicitly bound to a table that reaches an igw", "subnet-public", true},
		// The main table reaches an internet gateway, but an explicit association to a NAT-only table overrides it.
		{"explicitly bound to a table that does not", "subnet-private", false},
		{"unbound, so governed by the main table", "subnet-unbound", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reach.reaches(tc.subnetID, "vpc-1"); got != tc.want {
				t.Errorf("reaches(%s) = %v, want %v", tc.subnetID, got, tc.want)
			}
		})
	}
}

// A VPC whose main table has no internet gateway leaves its unbound subnets private, which is the safe default to get wrong in the right direction.
func TestPublicSubnetsDefaultsToPrivate(t *testing.T) {
	reach := publicSubnets([]RouteTable{
		{ID: "rtb-main", VpcID: "vpc-1", Main: true, Routes: []Route{{Destination: "10.0.0.0/16", Target: "local"}}},
	})

	if reach.reaches("subnet-unbound", "vpc-1") {
		t.Error("an unbound subnet under a private main table must not read as public")
	}
	if reach.reaches("subnet-unbound", "vpc-unknown") {
		t.Error("a subnet in a VPC with no known tables must not read as public")
	}
}

// Only an internet gateway makes a subnet publicly reachable; a NAT or transit gateway on the same route must not.
func TestPublicSubnetsIgnoresOtherGatewayKinds(t *testing.T) {
	for _, target := range []string{"nat-0a1b", "tgw-0a1b", "pcx-0a1b", "vgw-0a1b", "eigw-0a1b", ""} {
		reach := publicSubnets([]RouteTable{
			{ID: "rtb", VpcID: "vpc-1", SubnetIDs: []string{"subnet-a"}, Routes: []Route{{Destination: "0.0.0.0/0", Target: target}}},
		})
		if reach.reaches("subnet-a", "vpc-1") {
			t.Errorf("target %q must not mark a subnet public", target)
		}
	}
}

func TestRouteTargetPrefersTheSetField(t *testing.T) {
	cases := []struct {
		name  string
		route types.Route
		want  string
	}{
		{"internet gateway", types.Route{GatewayId: aws.String("igw-1")}, "igw-1"},
		{"nat gateway", types.Route{NatGatewayId: aws.String("nat-1")}, "nat-1"},
		{"transit gateway", types.Route{TransitGatewayId: aws.String("tgw-1")}, "tgw-1"},
		{"peering connection", types.Route{VpcPeeringConnectionId: aws.String("pcx-1")}, "pcx-1"},
		{"network interface", types.Route{NetworkInterfaceId: aws.String("eni-1")}, "eni-1"},
		{"nothing set", types.Route{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := routeTarget(tc.route); got != tc.want {
				t.Errorf("routeTarget() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRouteDestinationFallsBackThroughItsForms(t *testing.T) {
	cases := []struct {
		name  string
		route types.Route
		want  string
	}{
		{"ipv4", types.Route{DestinationCidrBlock: aws.String("0.0.0.0/0")}, "0.0.0.0/0"},
		{"ipv6", types.Route{DestinationIpv6CidrBlock: aws.String("::/0")}, "::/0"},
		{"prefix list", types.Route{DestinationPrefixListId: aws.String("pl-1")}, "pl-1"},
		{"nothing set", types.Route{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := routeDestination(tc.route); got != tc.want {
				t.Errorf("routeDestination() = %q, want %q", got, tc.want)
			}
		})
	}
}

// Several EC2 describes end a walk with an empty token rather than a nil one, and treating that as another page loops forever.
func TestHasNextPage(t *testing.T) {
	if hasNextPage(nil) {
		t.Error("a nil token must end the walk")
	}
	if hasNextPage(aws.String("")) {
		t.Error("an empty token must end the walk")
	}
	if !hasNextPage(aws.String("more")) {
		t.Error("a populated token must continue the walk")
	}
}
