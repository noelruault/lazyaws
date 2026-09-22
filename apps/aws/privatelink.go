package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// VPCEndpointStatePendingAcceptance is the connection state that waits on a human, spelled as EC2 answers rather than as the SDK declares it.
// DescribeVpcEndpointConnections returns lowercase states while types.StatePendingAcceptance is "PendingAcceptance", so comparisons fold case and the filter sends both spellings.
const VPCEndpointStatePendingAcceptance = "pendingAcceptance"

// VPCEndpointService is a PrivateLink service this account publishes, which is the provider side of a VPC endpoint: consumers create endpoints against it, and when AcceptanceRequired is set each one waits until this account accepts it.
type VPCEndpointService struct {
	ID                  string
	Name                string
	NameTag             string
	State               string
	AcceptanceRequired  bool
	ManagesEndpoints    bool
	PrivateDNSName      string
	PayerResponsibility string
	AvailabilityZones   []string
	BaseDNSNames        []string
	LoadBalancerARNs    []string
	SupportedIPTypes    []string
	Tags                []Tag

	// Pending counts the connections in pendingAcceptance, filled by ListVPCEndpointServices rather than by DescribeVpcEndpointServiceConfigurations, which does not report it.
	Pending int
}

// Label is what identifies the service to a person: the Name tag when it has one, and the service id otherwise, because the service name is a generated string that repeats the id.
func (s VPCEndpointService) Label() string {
	if s.NameTag != "" {
		return s.NameTag
	}
	return s.ID
}

// VPCEndpointConnection is one consumer endpoint attached to a service this account owns.
type VPCEndpointConnection struct {
	ServiceID        string
	EndpointID       string
	ConnectionID     string
	Owner            string
	State            string
	Region           string
	IPAddressType    string
	CreatedAt        *time.Time
	DNSNames         []string
	LoadBalancerARNs []string
	Tags             []Tag
}

// Pending reports whether this connection is waiting for this account to accept or reject it.
// It decides whether the Accept action is offered at all, so it folds case: see VPCEndpointStatePendingAcceptance for the two spellings in play.
func (c VPCEndpointConnection) Pending() bool {
	return strings.EqualFold(c.State, VPCEndpointStatePendingAcceptance)
}

// ListVPCEndpointServices returns the PrivateLink services this account publishes in the region, each carrying the number of connections waiting on it.
// The pending count costs one extra describe for the whole list rather than one per service, because the connections describe is filtered by state and not by service: the count is the reason to open this panel at all, so the list carries it rather than making each row ask.
func (c *Client) ListVPCEndpointServices(ctx context.Context) ([]VPCEndpointService, error) {
	input := &ec2.DescribeVpcEndpointServiceConfigurationsInput{}
	var services []VPCEndpointService
	for {
		result, err := c.EC2.DescribeVpcEndpointServiceConfigurations(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to describe vpc endpoint service configurations: %w", err)
		}

		for _, service := range result.ServiceConfigurations {
			services = append(services, newVPCEndpointService(service))
		}

		if !hasNextPage(result.NextToken) {
			break
		}
		input.NextToken = result.NextToken
	}

	if len(services) == 0 {
		return nil, nil
	}

	// A failed count fails the list: a row reading "0 pending" when the count could not be made would be the one lie this panel must not tell.
	pending, err := c.countPendingConnections(ctx)
	if err != nil {
		return nil, err
	}
	for i := range services {
		services[i].Pending = pending[services[i].ID]
	}

	return services, nil
}

// countPendingConnections returns pending connections per service id, asking EC2 only for the state that matters.
func (c *Client) countPendingConnections(ctx context.Context) (map[string]int, error) {
	// Both spellings go in because a filter value EC2 does not recognise matches nothing rather than failing, which would report zero waiting connections instead of an error.
	input := &ec2.DescribeVpcEndpointConnectionsInput{
		Filters: []types.Filter{{
			Name:   awssdk.String("vpc-endpoint-state"),
			Values: []string{VPCEndpointStatePendingAcceptance, "PendingAcceptance"},
		}},
	}

	pending := map[string]int{}
	for {
		result, err := c.EC2.DescribeVpcEndpointConnections(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to count pending vpc endpoint connections: %w", err)
		}

		for _, connection := range result.VpcEndpointConnections {
			pending[getString(connection.ServiceId)]++
		}

		if !hasNextPage(result.NextToken) {
			break
		}
		input.NextToken = result.NextToken
	}

	return pending, nil
}

// ListVPCEndpointConnections returns every connection to one service, in every state, because a rejected or failed connection is what explains a consumer's ticket as often as a pending one does.
func (c *Client) ListVPCEndpointConnections(ctx context.Context, serviceID string) ([]VPCEndpointConnection, error) {
	input := &ec2.DescribeVpcEndpointConnectionsInput{Filters: serviceFilter(serviceID)}
	var connections []VPCEndpointConnection
	for {
		result, err := c.EC2.DescribeVpcEndpointConnections(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to describe vpc endpoint connections: %w", err)
		}

		for _, connection := range result.VpcEndpointConnections {
			connections = append(connections, newVPCEndpointConnection(connection))
		}

		if !hasNextPage(result.NextToken) {
			break
		}
		input.NextToken = result.NextToken
	}

	return connections, nil
}

// AcceptVPCEndpointConnections accepts consumer endpoints on a service this account owns, which is what moves them out of pendingAcceptance and lets traffic flow.
func (c *Client) AcceptVPCEndpointConnections(ctx context.Context, serviceID string, endpointIDs []string) error {
	if err := checkConnectionTargets(serviceID, endpointIDs); err != nil {
		return err
	}

	result, err := c.EC2.AcceptVpcEndpointConnections(ctx, &ec2.AcceptVpcEndpointConnectionsInput{
		ServiceId:      awssdk.String(serviceID),
		VpcEndpointIds: endpointIDs,
	})
	if err != nil {
		return fmt.Errorf("failed to accept vpc endpoint connections: %w", err)
	}

	return unsuccessfulError("accept", result.Unsuccessful)
}

// RejectVPCEndpointConnections rejects consumer endpoints on a service this account owns.
// A rejected endpoint is not deleted: it stays on the consumer's side in the rejected state, and the consumer can request again.
func (c *Client) RejectVPCEndpointConnections(ctx context.Context, serviceID string, endpointIDs []string) error {
	if err := checkConnectionTargets(serviceID, endpointIDs); err != nil {
		return err
	}

	result, err := c.EC2.RejectVpcEndpointConnections(ctx, &ec2.RejectVpcEndpointConnectionsInput{
		ServiceId:      awssdk.String(serviceID),
		VpcEndpointIds: endpointIDs,
	})
	if err != nil {
		return fmt.Errorf("failed to reject vpc endpoint connections: %w", err)
	}

	return unsuccessfulError("reject", result.Unsuccessful)
}

func checkConnectionTargets(serviceID string, endpointIDs []string) error {
	if serviceID == "" {
		return fmt.Errorf("no endpoint service given")
	}
	if len(endpointIDs) == 0 {
		return fmt.Errorf("no endpoints given for %s", serviceID)
	}
	return nil
}

// unsuccessfulError turns the per-endpoint failures EC2 reports inside a successful response into an error.
// Accept and Reject answer 200 with an Unsuccessful list, so a call that changed nothing looks like a call that worked; without this the UI would report success and the connection would still be pending.
func unsuccessfulError(action string, items []types.UnsuccessfulItem) error {
	if len(items) == 0 {
		return nil
	}

	reasons := make([]string, len(items))
	for i, item := range items {
		reason := getString(item.ResourceId)
		if item.Error != nil {
			reason = strings.TrimSpace(reason + ": " + getString(item.Error.Code) + " " + getString(item.Error.Message))
		}
		reasons[i] = reason
	}

	return fmt.Errorf("failed to %s %d endpoint(s): %s", action, len(items), strings.Join(reasons, "; "))
}

func serviceFilter(serviceID string) []types.Filter {
	if serviceID == "" {
		return nil
	}
	return []types.Filter{{Name: awssdk.String("service-id"), Values: []string{serviceID}}}
}

func newVPCEndpointService(s types.ServiceConfiguration) VPCEndpointService {
	service := VPCEndpointService{
		ID:                  getString(s.ServiceId),
		Name:                getString(s.ServiceName),
		NameTag:             getNameTag(s.Tags),
		State:               string(s.ServiceState),
		AcceptanceRequired:  s.AcceptanceRequired != nil && *s.AcceptanceRequired,
		ManagesEndpoints:    s.ManagesVpcEndpoints != nil && *s.ManagesVpcEndpoints,
		PrivateDNSName:      getString(s.PrivateDnsName),
		PayerResponsibility: string(s.PayerResponsibility),
		AvailabilityZones:   s.AvailabilityZones,
		BaseDNSNames:        s.BaseEndpointDnsNames,
		Tags:                toTags(s.Tags),
	}

	// Gateway Load Balancer services report their balancers in a second field, and a service has one kind or the other; both are the same thing to a reader asking what sits behind the service.
	service.LoadBalancerARNs = append(append([]string(nil), s.NetworkLoadBalancerArns...), s.GatewayLoadBalancerArns...)

	for _, ipType := range s.SupportedIpAddressTypes {
		service.SupportedIPTypes = append(service.SupportedIPTypes, string(ipType))
	}

	return service
}

func newVPCEndpointConnection(c types.VpcEndpointConnection) VPCEndpointConnection {
	connection := VPCEndpointConnection{
		ServiceID:     getString(c.ServiceId),
		EndpointID:    getString(c.VpcEndpointId),
		ConnectionID:  getString(c.VpcEndpointConnectionId),
		Owner:         getString(c.VpcEndpointOwner),
		State:         string(c.VpcEndpointState),
		Region:        getString(c.VpcEndpointRegion),
		IPAddressType: string(c.IpAddressType),
		CreatedAt:     c.CreationTimestamp,
		Tags:          toTags(c.Tags),
	}

	connection.LoadBalancerARNs = append(append([]string(nil), c.NetworkLoadBalancerArns...), c.GatewayLoadBalancerArns...)

	for _, entry := range c.DnsEntries {
		if name := getString(entry.DnsName); name != "" {
			connection.DNSNames = append(connection.DNSNames, name)
		}
	}

	return connection
}
