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

type VPC struct {
	ID             string
	Name           string
	State          string
	CIDR           string
	Tenancy        string
	OwnerID        string
	DHCPOptionsID  string
	IsDefault      bool
	SecondaryCIDRs []string
	IPv6CIDRs      []string
	Tags           []Tag

	// DNS resolution and hostnames are VPC attributes rather than fields of the VPC itself, so these stay false until GetVPCDNS fills them.
	DNSSupport   bool
	DNSHostnames bool
}

type Subnet struct {
	ID                  string
	Name                string
	State               string
	CIDR                string
	AZ                  string
	VpcID               string
	OwnerID             string
	AvailableIPs        int32
	MapPublicIPOnLaunch bool
	// Public reports whether the route table governing this subnet reaches an internet gateway; see publicSubnets.
	Public bool
	Tags   []Tag
}

type RouteTable struct {
	ID        string
	Name      string
	VpcID     string
	OwnerID   string
	Main      bool
	Routes    []Route
	SubnetIDs []string
	Tags      []Tag
}

type Route struct {
	Destination string
	Target      string
	State       string
	Origin      string
}

type InternetGateway struct {
	ID      string
	Name    string
	VpcID   string
	State   string
	OwnerID string
	Tags    []Tag
}

type NATGateway struct {
	ID               string
	Name             string
	State            string
	ConnectivityType string
	VpcID            string
	SubnetID         string
	PublicIP         string
	PrivateIP        string
	FailureMessage   string
	CreatedAt        *time.Time
	Tags             []Tag
}

type TransitGateway struct {
	ID                     string
	Name                   string
	ARN                    string
	State                  string
	OwnerID                string
	Description            string
	AmazonSideASN          int64
	AssociationRouteTable  string
	PropagationRouteTable  string
	DNSSupport             string
	VPNECMPSupport         string
	MulticastSupport       string
	AutoAcceptSharedAttach string
	CreatedAt              *time.Time
	Tags                   []Tag
}

type TGWAttachment struct {
	ID               string
	Name             string
	TransitGatewayID string
	TGWOwnerID       string
	ResourceType     string
	ResourceID       string
	ResourceOwnerID  string
	State            string
	RouteTableID     string
	AssociationState string
	CreatedAt        *time.Time
	Tags             []Tag
}

func (c *Client) ListVPCs(ctx context.Context) ([]VPC, error) {
	var vpcs []VPC
	input := &ec2.DescribeVpcsInput{}
	for {
		result, err := c.EC2.DescribeVpcs(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to describe vpcs: %w", err)
		}

		for _, v := range result.Vpcs {
			vpc := VPC{
				ID:            getString(v.VpcId),
				Name:          getNameTag(v.Tags),
				State:         string(v.State),
				CIDR:          getString(v.CidrBlock),
				Tenancy:       string(v.InstanceTenancy),
				OwnerID:       getString(v.OwnerId),
				DHCPOptionsID: getString(v.DhcpOptionsId),
				IsDefault:     v.IsDefault != nil && *v.IsDefault,
				Tags:          toTags(v.Tags),
			}

			// The primary block also appears in the association set, so listing it again as secondary would double-count it.
			for _, assoc := range v.CidrBlockAssociationSet {
				if cidr := getString(assoc.CidrBlock); cidr != "" && cidr != vpc.CIDR {
					vpc.SecondaryCIDRs = append(vpc.SecondaryCIDRs, cidr)
				}
			}
			for _, assoc := range v.Ipv6CidrBlockAssociationSet {
				if cidr := getString(assoc.Ipv6CidrBlock); cidr != "" {
					vpc.IPv6CIDRs = append(vpc.IPv6CIDRs, cidr)
				}
			}

			vpcs = append(vpcs, vpc)
		}

		if !hasNextPage(result.NextToken) {
			break
		}
		input.NextToken = result.NextToken
	}

	return vpcs, nil
}

// GetVPCDNS reads the two DNS attributes, which DescribeVpcs omits and which only DescribeVpcAttribute can return, one call per attribute.
func (c *Client) GetVPCDNS(ctx context.Context, vpcID string) (support bool, hostnames bool, err error) {
	read := func(attribute types.VpcAttributeName) (bool, error) {
		out, err := c.EC2.DescribeVpcAttribute(ctx, &ec2.DescribeVpcAttributeInput{
			VpcId:     awssdk.String(vpcID),
			Attribute: attribute,
		})
		if err != nil {
			return false, err
		}
		switch attribute {
		case types.VpcAttributeNameEnableDnsSupport:
			return out.EnableDnsSupport != nil && out.EnableDnsSupport.Value != nil && *out.EnableDnsSupport.Value, nil
		default:
			return out.EnableDnsHostnames != nil && out.EnableDnsHostnames.Value != nil && *out.EnableDnsHostnames.Value, nil
		}
	}

	if support, err = read(types.VpcAttributeNameEnableDnsSupport); err != nil {
		return false, false, fmt.Errorf("failed to read dns support for %s: %w", vpcID, err)
	}
	if hostnames, err = read(types.VpcAttributeNameEnableDnsHostnames); err != nil {
		return false, false, fmt.Errorf("failed to read dns hostnames for %s: %w", vpcID, err)
	}

	return support, hostnames, nil
}

// ListSubnets also reads the VPC's route tables, because whether a subnet is public is a property of the table governing it rather than of the subnet.
func (c *Client) ListSubnets(ctx context.Context, vpcID string) ([]Subnet, error) {
	tables, err := c.ListRouteTables(ctx, vpcID)
	if err != nil {
		return nil, err
	}
	public := publicSubnets(tables)

	var subnets []Subnet
	input := &ec2.DescribeSubnetsInput{Filters: vpcFilter(vpcID)}
	for {
		result, err := c.EC2.DescribeSubnets(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to describe subnets: %w", err)
		}

		for _, s := range result.Subnets {
			subnet := Subnet{
				ID:                  getString(s.SubnetId),
				Name:                getNameTag(s.Tags),
				State:               string(s.State),
				CIDR:                getString(s.CidrBlock),
				AZ:                  getString(s.AvailabilityZone),
				VpcID:               getString(s.VpcId),
				OwnerID:             getString(s.OwnerId),
				MapPublicIPOnLaunch: s.MapPublicIpOnLaunch != nil && *s.MapPublicIpOnLaunch,
				Tags:                toTags(s.Tags),
			}
			if s.AvailableIpAddressCount != nil {
				subnet.AvailableIPs = *s.AvailableIpAddressCount
			}
			subnet.Public = public.reaches(subnet.ID, subnet.VpcID)

			subnets = append(subnets, subnet)
		}

		if !hasNextPage(result.NextToken) {
			break
		}
		input.NextToken = result.NextToken
	}

	return subnets, nil
}

func (c *Client) ListRouteTables(ctx context.Context, vpcID string) ([]RouteTable, error) {
	var tables []RouteTable
	input := &ec2.DescribeRouteTablesInput{Filters: vpcFilter(vpcID)}
	for {
		result, err := c.EC2.DescribeRouteTables(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to describe route tables: %w", err)
		}

		for _, t := range result.RouteTables {
			table := RouteTable{
				ID:      getString(t.RouteTableId),
				Name:    getNameTag(t.Tags),
				VpcID:   getString(t.VpcId),
				OwnerID: getString(t.OwnerId),
				Tags:    toTags(t.Tags),
			}

			for _, assoc := range t.Associations {
				if assoc.Main != nil && *assoc.Main {
					table.Main = true
				}
				if id := getString(assoc.SubnetId); id != "" {
					table.SubnetIDs = append(table.SubnetIDs, id)
				}
			}

			for _, r := range t.Routes {
				table.Routes = append(table.Routes, Route{
					Destination: routeDestination(r),
					Target:      routeTarget(r),
					State:       string(r.State),
					Origin:      string(r.Origin),
				})
			}

			tables = append(tables, table)
		}

		if !hasNextPage(result.NextToken) {
			break
		}
		input.NextToken = result.NextToken
	}

	return tables, nil
}

func (c *Client) ListInternetGateways(ctx context.Context, vpcID string) ([]InternetGateway, error) {
	input := &ec2.DescribeInternetGatewaysInput{}
	// An internet gateway names its VPC through an attachment, so it is not selected by the plain vpc-id filter the other calls use.
	if vpcID != "" {
		input.Filters = []types.Filter{{Name: awssdk.String("attachment.vpc-id"), Values: []string{vpcID}}}
	}

	var gateways []InternetGateway
	for {
		result, err := c.EC2.DescribeInternetGateways(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to describe internet gateways: %w", err)
		}

		for _, g := range result.InternetGateways {
			gateway := InternetGateway{
				ID:      getString(g.InternetGatewayId),
				Name:    getNameTag(g.Tags),
				OwnerID: getString(g.OwnerId),
				State:   "detached",
				Tags:    toTags(g.Tags),
			}
			// A gateway attaches to at most one VPC, so the first attachment is the whole story.
			for _, attachment := range g.Attachments {
				gateway.VpcID = getString(attachment.VpcId)
				gateway.State = string(attachment.State)
				break
			}

			gateways = append(gateways, gateway)
		}

		if !hasNextPage(result.NextToken) {
			break
		}
		input.NextToken = result.NextToken
	}

	return gateways, nil
}

func (c *Client) ListNATGateways(ctx context.Context, vpcID string) ([]NATGateway, error) {
	var gateways []NATGateway
	input := &ec2.DescribeNatGatewaysInput{Filter: vpcFilter(vpcID)}
	for {
		result, err := c.EC2.DescribeNatGateways(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to describe nat gateways: %w", err)
		}

		for _, g := range result.NatGateways {
			gateway := NATGateway{
				ID:               getString(g.NatGatewayId),
				Name:             getNameTag(g.Tags),
				State:            string(g.State),
				ConnectivityType: string(g.ConnectivityType),
				VpcID:            getString(g.VpcId),
				SubnetID:         getString(g.SubnetId),
				FailureMessage:   getString(g.FailureMessage),
				CreatedAt:        g.CreateTime,
				Tags:             toTags(g.Tags),
			}
			// A private NAT gateway has no public address at all, so both are read independently rather than from one entry.
			for _, address := range g.NatGatewayAddresses {
				if gateway.PublicIP == "" {
					gateway.PublicIP = getString(address.PublicIp)
				}
				if gateway.PrivateIP == "" {
					gateway.PrivateIP = getString(address.PrivateIp)
				}
			}

			gateways = append(gateways, gateway)
		}

		if !hasNextPage(result.NextToken) {
			break
		}
		input.NextToken = result.NextToken
	}

	return gateways, nil
}

func (c *Client) ListTransitGateways(ctx context.Context) ([]TransitGateway, error) {
	var gateways []TransitGateway
	input := &ec2.DescribeTransitGatewaysInput{}
	for {
		result, err := c.EC2.DescribeTransitGateways(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to describe transit gateways: %w", err)
		}

		for _, g := range result.TransitGateways {
			gateway := TransitGateway{
				ID:          getString(g.TransitGatewayId),
				Name:        getNameTag(g.Tags),
				ARN:         getString(g.TransitGatewayArn),
				State:       string(g.State),
				OwnerID:     getString(g.OwnerId),
				Description: getString(g.Description),
				CreatedAt:   g.CreationTime,
				Tags:        toTags(g.Tags),
			}
			if o := g.Options; o != nil {
				if o.AmazonSideAsn != nil {
					gateway.AmazonSideASN = *o.AmazonSideAsn
				}
				gateway.AssociationRouteTable = getString(o.AssociationDefaultRouteTableId)
				gateway.PropagationRouteTable = getString(o.PropagationDefaultRouteTableId)
				gateway.DNSSupport = string(o.DnsSupport)
				gateway.VPNECMPSupport = string(o.VpnEcmpSupport)
				gateway.MulticastSupport = string(o.MulticastSupport)
				gateway.AutoAcceptSharedAttach = string(o.AutoAcceptSharedAttachments)
			}

			gateways = append(gateways, gateway)
		}

		if !hasNextPage(result.NextToken) {
			break
		}
		input.NextToken = result.NextToken
	}

	return gateways, nil
}

// ListTGWAttachments returns every attachment in the region; attachments are how a VPC reaches a transit gateway, and the reverse lookup is by ResourceID.
func (c *Client) ListTGWAttachments(ctx context.Context) ([]TGWAttachment, error) {
	var attachments []TGWAttachment
	input := &ec2.DescribeTransitGatewayAttachmentsInput{}
	for {
		result, err := c.EC2.DescribeTransitGatewayAttachments(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to describe transit gateway attachments: %w", err)
		}

		for _, a := range result.TransitGatewayAttachments {
			attachment := TGWAttachment{
				ID:               getString(a.TransitGatewayAttachmentId),
				Name:             getNameTag(a.Tags),
				TransitGatewayID: getString(a.TransitGatewayId),
				TGWOwnerID:       getString(a.TransitGatewayOwnerId),
				ResourceType:     string(a.ResourceType),
				ResourceID:       getString(a.ResourceId),
				ResourceOwnerID:  getString(a.ResourceOwnerId),
				State:            string(a.State),
				CreatedAt:        a.CreationTime,
				Tags:             toTags(a.Tags),
			}
			if a.Association != nil {
				attachment.RouteTableID = getString(a.Association.TransitGatewayRouteTableId)
				attachment.AssociationState = string(a.Association.State)
			}

			attachments = append(attachments, attachment)
		}

		if !hasNextPage(result.NextToken) {
			break
		}
		input.NextToken = result.NextToken
	}

	return attachments, nil
}

// VPCEndpoint carries every field the panel renders because DescribeVpcEndpoints returns the whole record; there is no second describe call to make.
type VPCEndpoint struct {
	ID                  string
	ServiceName         string
	Type                string
	State               string
	VpcID               string
	OwnerID             string
	Name                string
	IPAddressType       string
	PrivateDNSEnabled   bool
	RequesterManaged    bool
	CreatedAt           *time.Time
	SubnetIDs           []string
	RouteTableIDs       []string
	NetworkInterfaceIDs []string
	SecurityGroups      []SecurityGroup
	DNSNames            []string
	FailureReason       string
	LastError           string
	PolicyDocument      string
	Tags                []Tag
}

// ShortService drops the com.amazonaws.<region> prefix every endpoint carries, leaving the part that names the service.
// A PrivateLink service owned by another account inserts a "vpce" segment ahead of the region, so it has one more to drop.
func (e VPCEndpoint) ShortService() string {
	parts := strings.Split(e.ServiceName, ".")
	if len(parts) < 4 || parts[0] != "com" || parts[1] != "amazonaws" {
		return e.ServiceName
	}

	drop := 3
	if parts[2] == "vpce" {
		drop = 4
	}
	if len(parts) <= drop {
		return e.ServiceName
	}

	return strings.Join(parts[drop:], ".")
}

func (c *Client) ListVPCEndpoints(ctx context.Context, vpcID string) ([]VPCEndpoint, error) {
	input := &ec2.DescribeVpcEndpointsInput{Filters: vpcFilter(vpcID)}
	var endpoints []VPCEndpoint
	for {
		result, err := c.EC2.DescribeVpcEndpoints(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to describe vpc endpoints: %w", err)
		}

		for _, endpoint := range result.VpcEndpoints {
			endpoints = append(endpoints, newVPCEndpoint(endpoint))
		}

		if !hasNextPage(result.NextToken) {
			break
		}
		input.NextToken = result.NextToken
	}

	return endpoints, nil
}

func newVPCEndpoint(e types.VpcEndpoint) VPCEndpoint {
	endpoint := VPCEndpoint{
		ID:                  getString(e.VpcEndpointId),
		ServiceName:         getString(e.ServiceName),
		Type:                string(e.VpcEndpointType),
		State:               string(e.State),
		VpcID:               getString(e.VpcId),
		OwnerID:             getString(e.OwnerId),
		Name:                getNameTag(e.Tags),
		IPAddressType:       string(e.IpAddressType),
		PrivateDNSEnabled:   e.PrivateDnsEnabled != nil && *e.PrivateDnsEnabled,
		RequesterManaged:    e.RequesterManaged != nil && *e.RequesterManaged,
		CreatedAt:           e.CreationTimestamp,
		SubnetIDs:           e.SubnetIds,
		RouteTableIDs:       e.RouteTableIds,
		NetworkInterfaceIDs: e.NetworkInterfaceIds,
		FailureReason:       getString(e.FailureReason),
		PolicyDocument:      getString(e.PolicyDocument),
		Tags:                toTags(e.Tags),
	}

	for _, group := range e.Groups {
		endpoint.SecurityGroups = append(endpoint.SecurityGroups, SecurityGroup{
			ID:   getString(group.GroupId),
			Name: getString(group.GroupName),
		})
	}

	for _, entry := range e.DnsEntries {
		if name := getString(entry.DnsName); name != "" {
			endpoint.DNSNames = append(endpoint.DNSNames, name)
		}
	}

	if e.LastError != nil {
		endpoint.LastError = strings.TrimSpace(getString(e.LastError.Code) + " " + getString(e.LastError.Message))
	}

	return endpoint
}

// routeReach records, per route table, whether that table can reach an internet gateway, keyed both by the subnets bound to it and by the VPC whose main table it is.
type routeReach struct {
	bySubnet map[string]bool
	byVPC    map[string]bool
}

// reaches resolves a subnet to the table that governs it: the one explicitly associated with it, or its VPC's main table when nothing is.
func (r routeReach) reaches(subnetID, vpcID string) bool {
	if public, bound := r.bySubnet[subnetID]; bound {
		return public
	}
	return r.byVPC[vpcID]
}

// publicSubnets classifies a VPC's route tables by whether they carry a route to an internet gateway, which is what makes the subnets under them publicly reachable.
func publicSubnets(tables []RouteTable) routeReach {
	reach := routeReach{bySubnet: map[string]bool{}, byVPC: map[string]bool{}}

	for _, table := range tables {
		open := false
		for _, route := range table.Routes {
			if strings.HasPrefix(route.Target, "igw-") {
				open = true
				break
			}
		}

		for _, subnetID := range table.SubnetIDs {
			reach.bySubnet[subnetID] = open
		}
		if table.Main {
			reach.byVPC[table.VpcID] = open
		}
	}

	return reach
}

// routeDestination reports what a route matches on: a CIDR, an IPv6 CIDR, or a managed prefix list.
func routeDestination(r types.Route) string {
	for _, candidate := range []*string{r.DestinationCidrBlock, r.DestinationIpv6CidrBlock, r.DestinationPrefixListId} {
		if value := getString(candidate); value != "" {
			return value
		}
	}
	return ""
}

// routeTarget reports where a route sends traffic. Exactly one target field is set per route, and the id prefix identifies the kind, so no separate type column is needed.
func routeTarget(r types.Route) string {
	for _, candidate := range []*string{
		r.GatewayId,
		r.NatGatewayId,
		r.TransitGatewayId,
		r.VpcPeeringConnectionId,
		r.NetworkInterfaceId,
		r.EgressOnlyInternetGatewayId,
		r.CarrierGatewayId,
		r.LocalGatewayId,
		r.InstanceId,
		r.CoreNetworkArn,
	} {
		if value := getString(candidate); value != "" {
			return value
		}
	}
	return ""
}

func vpcFilter(vpcID string) []types.Filter {
	if vpcID == "" {
		return nil
	}
	return []types.Filter{{Name: awssdk.String("vpc-id"), Values: []string{vpcID}}}
}

// hasNextPage treats the empty token as exhaustion because several EC2 describes end a walk with "" rather than nil.
func hasNextPage(token *string) bool {
	return token != nil && *token != ""
}

func toTags(tags []types.Tag) []Tag {
	if len(tags) == 0 {
		return nil
	}

	out := make([]Tag, len(tags))
	for i, tag := range tags {
		out[i] = Tag{Key: getString(tag.Key), Value: getString(tag.Value)}
	}
	return out
}
