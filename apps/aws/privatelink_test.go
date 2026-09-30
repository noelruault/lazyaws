package aws

import (
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestNewVPCEndpointServiceReadsEveryField(t *testing.T) {
	service := newVPCEndpointService(types.ServiceConfiguration{
		ServiceId:               aws.String("vpce-svc-0123456789abcdef0"),
		ServiceName:             aws.String("com.amazonaws.vpce.eu-west-1.vpce-svc-0123456789abcdef0"),
		ServiceState:            types.ServiceStateAvailable,
		AcceptanceRequired:      aws.Bool(true),
		ManagesVpcEndpoints:     aws.Bool(false),
		PrivateDnsName:          aws.String("api.internal.example"),
		PayerResponsibility:     types.PayerResponsibilityServiceOwner,
		AvailabilityZones:       []string{"eu-west-1a", "eu-west-1b"},
		BaseEndpointDnsNames:    []string{"vpce-svc-0123456789abcdef0.eu-west-1.vpce.amazonaws.com"},
		NetworkLoadBalancerArns: []string{"arn:aws:elasticloadbalancing:eu-west-1:111122223333:loadbalancer/net/api-nlb/abc123"},
		GatewayLoadBalancerArns: []string{"arn:aws:elasticloadbalancing:eu-west-1:111122223333:loadbalancer/gwy/api-gwlb/def456"},
		SupportedIpAddressTypes: []types.ServiceConnectivityType{types.ServiceConnectivityTypeIpv4},
		Tags: []types.Tag{
			{Key: aws.String("Name"), Value: aws.String("payments")},
			{Key: aws.String("team"), Value: aws.String("platform")},
		},
	})

	if service.ID != "vpce-svc-0123456789abcdef0" || service.State != "Available" {
		t.Errorf("id/state = %q/%q", service.ID, service.State)
	}
	if !service.AcceptanceRequired || service.ManagesEndpoints {
		t.Errorf("acceptance/managed = %v/%v, want true/false", service.AcceptanceRequired, service.ManagesEndpoints)
	}
	if service.NameTag != "payments" || service.Label() != "payments" {
		t.Errorf("name tag = %q, label = %q, want both payments", service.NameTag, service.Label())
	}
	if service.PrivateDNSName != "api.internal.example" || len(service.BaseDNSNames) != 1 {
		t.Errorf("dns = %q / %v", service.PrivateDNSName, service.BaseDNSNames)
	}
	// Both balancer kinds land in one field: a service has one or the other.
	if len(service.LoadBalancerARNs) != 2 {
		t.Errorf("load balancers = %v, want the NLB and the GWLB", service.LoadBalancerARNs)
	}
	if len(service.SupportedIPTypes) != 1 || service.SupportedIPTypes[0] != "ipv4" {
		t.Errorf("ip types = %v", service.SupportedIPTypes)
	}
	if len(service.Tags) != 2 || service.PayerResponsibility != "ServiceOwner" {
		t.Errorf("tags = %v, payer = %q", service.Tags, service.PayerResponsibility)
	}
	if service.Pending != 0 {
		t.Errorf("pending = %d, want 0: the describe this reads does not report it", service.Pending)
	}
}

// A service with no Name tag still has to identify itself, because that is the row the list draws.
func TestVPCEndpointServiceLabelFallsBackToTheID(t *testing.T) {
	service := newVPCEndpointService(types.ServiceConfiguration{ServiceId: aws.String("vpce-svc-06757e7b9dbbcc998")})
	if got := service.Label(); got != "vpce-svc-06757e7b9dbbcc998" {
		t.Errorf("label = %q, want the service id", got)
	}
}

func TestNewVPCEndpointConnectionReadsEveryField(t *testing.T) {
	created := time.Date(2026, 9, 17, 8, 30, 0, 0, time.UTC)
	connection := newVPCEndpointConnection(types.VpcEndpointConnection{
		ServiceId:               aws.String("vpce-svc-0123456789abcdef0"),
		VpcEndpointId:           aws.String("vpce-0123456789abcdef0"),
		VpcEndpointConnectionId: aws.String("vpce-con-01234567890abcdef"),
		VpcEndpointOwner:        aws.String("111122223333"),
		VpcEndpointState:        types.StatePendingAcceptance,
		VpcEndpointRegion:       aws.String("eu-west-1"),
		IpAddressType:           types.IpAddressTypeIpv4,
		CreationTimestamp:       &created,
		NetworkLoadBalancerArns: []string{"arn:aws:elasticloadbalancing:eu-west-1:111122223333:loadbalancer/net/api-nlb/abc123"},
		DnsEntries: []types.DnsEntry{
			{DnsName: aws.String("vpce-0ec31.eu-west-1.vpce.amazonaws.com")},
			{DnsName: nil},
			{DnsName: aws.String("")},
		},
		Tags: []types.Tag{{Key: aws.String("env"), Value: aws.String("staging")}},
	})

	if connection.EndpointID != "vpce-0123456789abcdef0" || connection.ServiceID != "vpce-svc-0123456789abcdef0" {
		t.Errorf("endpoint/service = %q/%q", connection.EndpointID, connection.ServiceID)
	}
	if connection.Owner != "111122223333" || connection.Region != "eu-west-1" || connection.IPAddressType != "ipv4" {
		t.Errorf("owner/region/ip = %q/%q/%q", connection.Owner, connection.Region, connection.IPAddressType)
	}
	if connection.CreatedAt == nil || !connection.CreatedAt.Equal(created) {
		t.Errorf("created = %v, want %v", connection.CreatedAt, created)
	}
	// A nil and an empty DNS name are both nothing to show, and neither may become a blank line in the detail pane.
	if len(connection.DNSNames) != 1 {
		t.Errorf("dns names = %v, want only the populated one", connection.DNSNames)
	}
	if len(connection.LoadBalancerARNs) != 1 || len(connection.Tags) != 1 {
		t.Errorf("balancers = %v, tags = %v", connection.LoadBalancerARNs, connection.Tags)
	}
}

// The SDK's State constants are NOT the strings EC2 answers with: DescribeVpcEndpointConnections returns lowercase states, verified against eu-west-1, while types.StatePendingAcceptance is "PendingAcceptance".
func TestPendingHoldsForBothSpellingsOfTheState(t *testing.T) {
	if string(types.StatePendingAcceptance) == VPCEndpointStatePendingAcceptance {
		t.Log("the SDK enum now matches the wire value; the case folding below is no longer load-bearing, but it is still correct")
	}

	for _, state := range []string{"pendingAcceptance", string(types.StatePendingAcceptance)} {
		if !(VPCEndpointConnection{State: state}).Pending() {
			t.Errorf("a connection in state %q does not report itself as pending", state)
		}
	}

	for _, state := range []string{"available", string(types.StateAvailable), "rejected", ""} {
		if (VPCEndpointConnection{State: state}).Pending() {
			t.Errorf("a connection in state %q reports itself as pending", state)
		}
	}
}

// AcceptVpcEndpointConnections answers 200 and reports per-endpoint failures inside the response, so without this a call that changed nothing would be reported as a success.
func TestUnsuccessfulItemsBecomeAnError(t *testing.T) {
	err := unsuccessfulError("accept", []types.UnsuccessfulItem{
		{
			ResourceId: aws.String("vpce-0123456789abcdef0"),
			Error: &types.UnsuccessfulItemError{
				Code:    aws.String("InvalidVpcEndpointId.NotFound"),
				Message: aws.String("The Vpc Endpoint Id does not exist"),
			},
		},
		{ResourceId: aws.String("vpce-0fedcba9876543210")},
	})
	if err == nil {
		t.Fatal("two failed endpoints reported no error")
	}

	for _, want := range []string{"accept", "vpce-0123456789abcdef0", "InvalidVpcEndpointId.NotFound", "does not exist", "vpce-0fedcba9876543210"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not carry %q", err, want)
		}
	}

	if err := unsuccessfulError("accept", nil); err != nil {
		t.Errorf("an empty Unsuccessful list produced %v, want nil", err)
	}
}

func TestConnectionTargetsAreChecked(t *testing.T) {
	if err := checkConnectionTargets("", []string{"vpce-1"}); err == nil {
		t.Error("a call with no service id was allowed")
	}
	if err := checkConnectionTargets("vpce-svc-1", nil); err == nil {
		t.Error("a call with no endpoints was allowed")
	}
	if err := checkConnectionTargets("vpce-svc-1", []string{"vpce-1"}); err != nil {
		t.Errorf("a complete call was refused: %v", err)
	}
}

func TestServiceFilterNamesTheServiceIDFilter(t *testing.T) {
	filters := serviceFilter("vpce-svc-086eb4005d4626dba")
	if len(filters) != 1 || getString(filters[0].Name) != "service-id" || filters[0].Values[0] != "vpce-svc-086eb4005d4626dba" {
		t.Errorf("filter = %+v, want one service-id filter", filters)
	}
	if serviceFilter("") != nil {
		t.Error("an empty service id produced a filter, which would match nothing instead of everything")
	}
}
