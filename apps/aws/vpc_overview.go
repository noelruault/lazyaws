package aws

import (
	"context"
	"errors"
)

// The VPCOverview.Errs keys, one per fetch. The subnet fetch also reads the route tables, because whether a subnet is public is a property of the table governing it rather than of the subnet.
const (
	SectionDNS       = "dns"
	SectionSubnets   = "subnets"
	SectionIGW       = "igw"
	SectionNAT       = "nat"
	SectionEndpoints = "endpoints"
)

// VPCOverview aggregates what the Config, Subnets, Gateways and Endpoints tabs each fetch separately.
// The VPC's own fields come off the list row; only the DNS attributes and the things attached to it need a call.
type VPCOverview struct {
	DNSSupport       bool
	DNSHostnames     bool
	Subnets          []Subnet
	InternetGateways []InternetGateway
	NATGateways      []NATGateway
	Endpoints        []VPCEndpoint

	Errs map[string]error
}

// Err reports the fetch error for a section, if that section failed.
func (o *VPCOverview) Err(section string) error {
	return o.Errs[section]
}

// GetVPCOverview fetches the VPC's attachments concurrently and always returns an overview, never an error.
// A section that failed is reported through Errs so the sections that succeeded still render: one denied describe degrades one block instead of blanking the pane.
func (c *Client) GetVPCOverview(ctx context.Context, vpcID string) *VPCOverview {
	overview := &VPCOverview{Errs: map[string]error{}}
	sections := newSectionFetcher(overview.Errs)

	sections.fetch(SectionDNS, c.vpcSection(func() (err error) {
		overview.DNSSupport, overview.DNSHostnames, err = c.GetVPCDNS(ctx, vpcID)
		return err
	}))
	sections.fetch(SectionSubnets, c.vpcSection(func() (err error) {
		overview.Subnets, err = c.ListSubnets(ctx, vpcID)
		return err
	}))
	sections.fetch(SectionIGW, c.vpcSection(func() (err error) {
		overview.InternetGateways, err = c.ListInternetGateways(ctx, vpcID)
		return err
	}))
	sections.fetch(SectionNAT, c.vpcSection(func() (err error) {
		overview.NATGateways, err = c.ListNATGateways(ctx, vpcID)
		return err
	}))
	sections.fetch(SectionEndpoints, c.vpcSection(func() (err error) {
		overview.Endpoints, err = c.ListVPCEndpoints(ctx, vpcID)
		return err
	}))

	sections.wait()

	return overview
}

// vpcSection runs a fetch behind the nil-client check none of the VPC describes carries, the same guard ec2.go added for the instance overview's fan-out.
// Guarding inside the fan-out rather than ahead of it is what keeps the failure per section, so a client with no EC2 reports five failed sections like any other outage.
func (c *Client) vpcSection(run func() error) func() error {
	return func() error {
		if c.EC2 == nil {
			return errors.New("EC2 client not initialized")
		}

		return run()
	}
}
