package presentation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// VPCWeights gives the label all the slack and sizes the vpc id to its content.
// A vpc id is a fixed 21 cells, so a proportional share of a wide panel under-pays it and cuts it while the label sits on idle padding; sizing it to content shows it whole wherever it fits, and the label's flexibleFloor is what keeps a row readable when it does not.
func VPCWeights() []int {
	return []int{0, 0, 1, 0}
}

// GetVPCDisplayCells leads with the CIDR because that is what a VPC gets recognised by when tracing whether two networks can reach each other; the name is a label on top of it, and often absent.
func GetVPCDisplayCells(v *aws.VPC) []utils.Cell {
	label := v.Name
	if label == "" {
		label = "(no name)"
	}
	if v.IsDefault {
		label += " (default)"
	}

	return []utils.Cell{
		StatusCellFit(v.State, StatusStyleIcon),
		{Text: v.CIDR, Color: color.Bold},
		{Text: label},
		// The vpc id is the fallback identifier: you read it when writing a rule or a query, not on every glance down the list.
		{Text: v.ID, Color: color.Faint},
	}
}

// FormatVPCOverview lays a VPC out for the Overview tab: the address space it owns on the left, and what is attached to it on the right.
// Everything but the DNS attributes comes off the list row or the tabs' own loaders, so the pane costs no call the Subnets, Gateways and Endpoints tabs do not already make.
func FormatVPCOverview(v *aws.VPC, o *aws.VPCOverview, width int) string {
	// Cut to the pane: the header spans the full width rather than a column, so Columns never measures it, and a long Name tag beside the CIDR and the id runs off the edge unmarked.
	// The state lives in the card row rather than as a badge beside the name (owner's call, 2026-08-28): one framed "available", not the same word twice.
	header := HeaderWithStats(width,
		ResourceHeader("VPC", vpcLabel(v), "", v.ID, v.CIDR, vpcDefaultNote(v)),
		vpcStatCards(v, o),
	)

	left := joinBlocks(vpcConfigBlock(v), vpcDNSBlock(o), vpcTagsBlock(v, ColumnWidth(width, overviewGap)))
	right := joinBlocks(vpcSubnetsBlock(o), vpcGatewaysBlock(o), vpcEndpointsBlock(o))

	return header + "\n\n" + Columns(width, overviewGap, left, right)
}

func vpcStatCards(v *aws.VPC, o *aws.VPCOverview) []Stat {
	public, freeIPs := vpcSubnetTotals(o.Subnets)
	subnets := utils.Cell{Text: fmt.Sprintf("%d · %d pub / %d priv", len(o.Subnets), public, len(o.Subnets)-public)}
	free := utils.Cell{Text: fmt.Sprintf("%d", freeIPs)}
	if o.Err(aws.SectionSubnets) != nil {
		subnets = utils.Cell{Text: "unavailable", Color: color.FgRed}
		free = utils.Cell{Text: "unavailable", Color: color.FgRed}
	}

	endpoints := utils.Cell{Text: fmt.Sprintf("%d", len(o.Endpoints))}
	if o.Err(aws.SectionEndpoints) != nil {
		endpoints = utils.Cell{Text: "unavailable", Color: color.FgRed}
	}

	return []Stat{
		{Label: "State", Value: BadgeCell(v.State)},
		{Label: "Subnets", Value: subnets},
		{Label: "Free IPs", Value: free},
		{Label: "Endpoints", Value: endpoints},
	}
}

func vpcSubnetTotals(subnets []aws.Subnet) (public int, freeIPs int32) {
	for _, subnet := range subnets {
		if subnet.Public {
			public++
		}
		freeIPs += subnet.AvailableIPs
	}

	return public, freeIPs
}

func vpcLabel(v *aws.VPC) string {
	if v.Name == "" {
		return "(no name)"
	}

	return v.Name
}

// vpcDefaultNote marks the default VPC, which is the one every account has and the one an unexpected resource usually turns out to be in.
func vpcDefaultNote(v *aws.VPC) string {
	if v.IsDefault {
		return "default VPC"
	}

	return ""
}

// vpcConfigBlock has no primary-CIDR row: the header carries it as the VPC's identity, and the same string twice was the dedup pass's finding.
func vpcConfigBlock(v *aws.VPC) string {
	rows := []kv{
		{"Secondary CIDRs", orNoneList(v.SecondaryCIDRs)},
		{"IPv6 CIDRs", orNoneList(v.IPv6CIDRs)},
		{"Default", yesNo(v.IsDefault)},
		{"Tenancy", orNone(v.Tenancy)},
		{"Owner", orNone(v.OwnerID)},
		{"DHCP options", orNone(v.DHCPOptionsID)},
	}

	return SectionTitle("Configuration") + "\n" + kvBlock(rows)
}

// vpcDNSBlock reads the overview rather than the VPC, because the two attributes are a separate describe and the list row leaves them false until it answers.
// Both default to on and are switched off deliberately, so an unreadable attribute must not render as the disabled state.
func vpcDNSBlock(o *aws.VPCOverview) string {
	if err := o.Err(aws.SectionDNS); err != nil {
		return sectionUnavailable("DNS", err)
	}

	rows := []kv{
		{"Resolution", onOff(o.DNSSupport)},
		{"Hostnames", onOff(o.DNSHostnames)},
	}

	return SectionTitle("DNS") + "\n" + kvBlock(rows)
}

func vpcSubnetsBlock(o *aws.VPCOverview) string {
	title := SectionTitle("Subnets")
	if err := o.Err(aws.SectionSubnets); err != nil {
		return sectionUnavailable("Subnets", err)
	}
	if len(o.Subnets) == 0 {
		return title + "\nnone"
	}

	// Only the AZ spread: the counts and the free-address total are the header's Subnets and Free IPs cards, and this section adds the one fact they cannot carry.
	byAZ := map[string]int{}
	for _, subnet := range o.Subnets {
		byAZ[subnet.AZ]++
	}

	return title + "\nAZs: " + countsByKey(byAZ)
}

// countsByKey renders a count per key in key order, since Go randomizes map iteration and an unsorted line would reshuffle itself on every re-render.
func countsByKey(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = fmt.Sprintf("%s (%d)", key, counts[key])
	}

	return strings.Join(parts, ", ")
}

func vpcGatewaysBlock(o *aws.VPCOverview) string {
	rows := []kv{
		{"Internet gateway", vpcIGWLine(o)},
		{"NAT gateways", vpcNATLine(o)},
	}

	return SectionTitle("Gateways") + "\n" + kvBlock(rows)
}

// vpcIGWLine answers whether the VPC can reach the internet at all, which is the fact every "why is this unreachable" question starts from.
func vpcIGWLine(o *aws.VPCOverview) string {
	if err := o.Err(aws.SectionIGW); err != nil {
		return fieldOr(err, "")
	}
	if len(o.InternetGateways) == 0 {
		return "none"
	}

	ids := make([]string, len(o.InternetGateways))
	for i, gateway := range o.InternetGateways {
		ids[i] = gateway.ID
	}

	return strings.Join(ids, ", ")
}

// vpcNATLine counts the NAT gateways by state rather than listing them: a NAT gateway is billed per hour, and how many are in which state is the question, not which id.
func vpcNATLine(o *aws.VPCOverview) string {
	if err := o.Err(aws.SectionNAT); err != nil {
		return fieldOr(err, "")
	}
	if len(o.NATGateways) == 0 {
		return "none"
	}

	byState := map[string]int{}
	for _, gateway := range o.NATGateways {
		byState[orNone(gateway.State)]++
	}

	return fmt.Sprintf("%d · %s", len(o.NATGateways), countsByKey(byState))
}

func vpcEndpointsBlock(o *aws.VPCOverview) string {
	title := SectionTitle("Endpoints")
	if err := o.Err(aws.SectionEndpoints); err != nil {
		return sectionUnavailable("Endpoints", err)
	}
	if len(o.Endpoints) == 0 {
		return title + "\nnone"
	}

	byType := map[string]int{}
	services := make([]string, 0, len(o.Endpoints))
	for _, endpoint := range o.Endpoints {
		byType[orNone(endpoint.Type)]++
		services = append(services, endpoint.ShortService())
	}
	sort.Strings(services)

	lines := []string{
		title,
		fmt.Sprintf("%s · %s", pluralize(len(o.Endpoints), "endpoint"), countsByKey(byType)),
		strings.Join(services, ", "),
	}

	return strings.Join(lines, "\n")
}

func vpcTagsBlock(v *aws.VPC, width int) string {
	title := SectionTitle("Tags")
	if len(v.Tags) == 0 {
		return title + "\nnone"
	}

	tags := make([]kv, len(v.Tags))
	for i, tag := range v.Tags {
		tags[i] = kv{tag.Key, tag.Value}
	}

	return title + "\n" + tagsBody(width, tags)
}

func orNoneList(values []string) string {
	if len(values) == 0 {
		return "none"
	}

	return strings.Join(values, ", ")
}

func onOff(b bool) string {
	if b {
		return "on"
	}

	return utils.ColoredString("off", color.FgYellow)
}

// GetVPCEndpointDisplayStrings labels the row by service rather than by id, because endpoints are usually untagged and one id looks like the next.
func GetVPCEndpointDisplayStrings(e *aws.VPCEndpoint) []string {
	label := e.Name
	if label == "" {
		label = e.ShortService()
	}

	return []string{
		StatusCell(e.State, StatusStyleIcon),
		label,
		e.ID,
		e.Type,
		e.VpcID,
	}
}
