package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/presentation"
	"github.com/noelruault/lazyaws/ui/tasks"
	"github.com/noelruault/lazyaws/ui/utils"
)

const vpcFetchTimeout = 20 * time.Second

func (gui *Gui) getVPCPanel() *panels.SideListPanel[*aws.VPC] {
	return &panels.SideListPanel[*aws.VPC]{
		ContextState: &panels.ContextState[*aws.VPC]{
			GetMainTabs: func() []panels.MainTab[*aws.VPC] {
				// No Config tab: the Overview's Configuration, DNS and Tags sections carry every field it held.
				return []panels.MainTab[*aws.VPC]{
					staticOverviewTab(gui, func(v *aws.VPC) string { return "vpc-" + v.ID }, gui.vpcOverview),
					{Key: "subnets", Title: "Subnets", Render: gui.renderVPCSubnets},
					{Key: "routes", Title: "Routes", Render: gui.renderVPCRoutes},
					{Key: "gateways", Title: "Gateways", Render: gui.renderVPCGateways},
					{
						Key:    "endpoints",
						Title:  "Endpoints",
						Render: gui.renderVPCEndpoints,
						Rows:   func(*aws.VPC) *panels.MainRows { return gui.vpcEndpointRows() },
					},
					{Key: "transit", Title: "Transit", Render: gui.renderVPCTransit},
				}
			},
			GetItemContextCacheKey: func(v *aws.VPC) string {
				return "vpc-" + v.ID
			},
		},

		ListPanel: panels.ListPanel[*aws.VPC]{
			List: panels.NewFilteredList[*aws.VPC](),
			View: gui.Views.VPC,
		},
		NoItemsMessage: "no VPCs",
		Gui:            gui.intoInterface(),

		// The default VPC sorts last: every account has one and it is rarely the one being investigated.
		Sort: func(a, b *aws.VPC) bool {
			if a.IsDefault != b.IsDefault {
				return b.IsDefault
			}
			return a.CIDR < b.CIDR
		},
		GetTableCellsFit: func(v *aws.VPC) []utils.Cell {
			return presentation.GetVPCDisplayCells(v)
		},
		Weights:   func(*aws.VPC) []int { return presentation.VPCWeights() },
		CopyValue: func(v *aws.VPC) string { return v.ID },
	}
}

func (gui *Gui) loadVPCList() error {
	if gui.Client == nil {
		return nil
	}

	gen := gui.Gen

	return gui.WithWaitingStatus("loading vpcs", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), vpcFetchTimeout)
		defer cancel()

		vpcs, err := gui.Client.ListVPCs(ctx)
		if err != nil {
			return err
		}
		if gen != gui.Gen {
			return nil
		}

		rows := make([]*aws.VPC, len(vpcs))
		for i := range vpcs {
			rows[i] = &vpcs[i]
		}
		gui.Panels.VPC.SetItemsKeepSelection(rows, vpcSelectionKey)
		return gui.Panels.VPC.RerenderList()
	})
}

// vpcSelectionKey identifies a VPC across reloads. The CIDR is not identity: VPCs in different regions, and peered ones, can share it.
func vpcSelectionKey(vpc *aws.VPC) string { return vpc.ID }

// vpcOverview consolidates the Config, Subnets, Gateways and Endpoints tabs, reading the VPC's own fields off the list row.
// Six EC2 describes against the tightest-throttled API this app touches is not a per-tick cost, and a VPC's topology is not a per-tick fact either, so the tab renders once per selection.
func (gui *Gui) vpcOverview(ctx context.Context, vpc *aws.VPC, width int) string {
	if gui.Client == nil {
		return overviewUnavailable("VPC")
	}

	fetchCtx, cancel := context.WithTimeout(ctx, vpcFetchTimeout)
	defer cancel()

	return presentation.FormatVPCOverview(vpc, gui.Client.GetVPCOverview(fetchCtx, vpc.ID), width)
}

// vpcTab runs one tab's fetch under the shared timeout and generation check, leaving each render below as only its query and its formatting.
func (gui *Gui) vpcTab(vpc *aws.VPC, render func(context.Context, string) (string, error)) tasks.TaskFunc {
	vpcID := vpc.ID
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, vpcFetchTimeout)
		defer cancel()

		out, err := render(fetchCtx, vpcID)
		if gen != gui.Gen {
			return
		}
		if err != nil {
			gui.RenderStringMain("error: " + err.Error())
			return
		}
		gui.RenderStringMain(out)
	}})
}

func (gui *Gui) renderVPCSubnets(vpc *aws.VPC) tasks.TaskFunc {
	return gui.vpcTab(vpc, func(ctx context.Context, vpcID string) (string, error) {
		subnets, err := gui.Client.ListSubnets(ctx, vpcID)
		if err != nil {
			return "", err
		}
		return formatVPCSubnets(subnets), nil
	})
}

func (gui *Gui) renderVPCRoutes(vpc *aws.VPC) tasks.TaskFunc {
	return gui.vpcTab(vpc, func(ctx context.Context, vpcID string) (string, error) {
		tables, err := gui.Client.ListRouteTables(ctx, vpcID)
		if err != nil {
			return "", err
		}
		return formatVPCRouteTables(tables), nil
	})
}

func (gui *Gui) renderVPCGateways(vpc *aws.VPC) tasks.TaskFunc {
	return gui.vpcTab(vpc, func(ctx context.Context, vpcID string) (string, error) {
		internet, err := gui.Client.ListInternetGateways(ctx, vpcID)
		if err != nil {
			return "", err
		}
		nat, err := gui.Client.ListNATGateways(ctx, vpcID)
		if err != nil {
			return "", err
		}
		return formatVPCGateways(internet, nat), nil
	})
}

// vpcEndpointsState keeps the last fetch so the main panel can address its rows, and remembers whether one record is open, because the whole record is already in hand and reopening it costs no call.
type vpcEndpointsState struct {
	vpcID     string
	endpoints []aws.VPCEndpoint
	showing   bool
	detail    int
}

func (gui *Gui) renderVPCEndpoints(vpc *aws.VPC) tasks.TaskFunc {
	return gui.vpcTab(vpc, func(ctx context.Context, vpcID string) (string, error) {
		endpoints, err := gui.Client.ListVPCEndpoints(ctx, vpcID)
		if err != nil {
			return "", err
		}

		// Moving to another VPC closes whatever record was open, since its index means nothing in the new list.
		if gui.vpcEndpoints.vpcID != vpcID {
			gui.vpcEndpoints = vpcEndpointsState{vpcID: vpcID}
		}
		gui.vpcEndpoints.endpoints = endpoints

		return gui.vpcEndpointsContent(), nil
	})
}

// vpcEndpointsContent renders whichever of the two views is current; both read the same in-memory fetch.
func (gui *Gui) vpcEndpointsContent() string {
	state := gui.vpcEndpoints
	if state.showing && state.detail >= 0 && state.detail < len(state.endpoints) {
		return formatVPCEndpointDetail(&state.endpoints[state.detail])
	}

	rows := gui.vpcEndpointRows()
	return renderMainRows(rows, gui.mainCursor(rows))
}

func (gui *Gui) rerenderVPCEndpoints() error {
	gui.reRenderStringMain(gui.vpcEndpointsContent())
	return nil
}

func (gui *Gui) vpcEndpointRows() *panels.MainRows {
	state := gui.vpcEndpoints

	// While a record is open there is nothing to walk, so the keys scroll it and Esc returns to the list.
	if state.showing {
		return &panels.MainRows{
			Back: func() error {
				gui.vpcEndpoints.showing = false
				return gui.rerenderVPCEndpoints()
			},
		}
	}

	cells := make([][]string, len(state.endpoints))
	for i := range state.endpoints {
		cells[i] = vpcEndpointRowCells(&state.endpoints[i])
	}

	return &panels.MainRows{
		EmptyMessage: "no endpoints in this vpc",
		Cells:        cells,
		Enter: func(i int) error {
			gui.vpcEndpoints.showing = true
			gui.vpcEndpoints.detail = i
			return gui.rerenderVPCEndpoints()
		},
		Actions: func(i int) error {
			return gui.vpcEndpointMenu(state.endpoints[i])
		},
	}
}

func vpcEndpointRowCells(e *aws.VPCEndpoint) []string {
	// Private DNS is meaningless on a gateway endpoint, so it reports neither enabled nor disabled there.
	privateDNS := "-"
	if e.Type == "Interface" {
		privateDNS = "private-dns " + formatEnabled(e.PrivateDNSEnabled)
	}

	return []string{
		presentation.StatusCell(e.State, presentation.StatusStyleIcon),
		e.ShortService(),
		e.ID,
		e.Type,
		privateDNS,
	}
}

// formatVPCEndpointDetail shows the fields DescribeVpcEndpoints already returned but the row had no width for.
func formatVPCEndpointDetail(e *aws.VPCEndpoint) string {
	fields := map[string]string{
		"ID":       e.ID,
		"Service":  e.ServiceName,
		"Type":     e.Type,
		"State":    e.State,
		"VPC":      e.VpcID,
		"Owner":    orDash(e.OwnerID),
		"Created":  formatSecretsTime(e.CreatedAt),
		"IP types": orDash(e.IPAddressType),
	}
	if e.Name != "" {
		fields["Name"] = e.Name
	}
	if e.Type == "Interface" {
		fields["Private DNS"] = formatEnabled(e.PrivateDNSEnabled)
	}
	if e.RequesterManaged {
		fields["Managed by"] = "the service, not this account"
	}
	if e.FailureReason != "" {
		fields["Failure"] = e.FailureReason
	}
	if e.LastError != "" {
		fields["Last error"] = e.LastError
	}

	securityGroups := make([]string, len(e.SecurityGroups))
	for i, group := range e.SecurityGroups {
		securityGroups[i] = fmt.Sprintf("%s (%s)", group.ID, group.Name)
	}

	out := utils.FormatMap(0, fields)
	out += formatVPCList("Subnets", e.SubnetIDs)
	out += formatVPCList("Route tables", e.RouteTableIDs)
	out += formatVPCList("Security groups", securityGroups)
	out += formatVPCList("Network interfaces", e.NetworkInterfaceIDs)
	out += formatVPCList("DNS names", e.DNSNames)
	out += formatVPCTags(e.Tags)
	out += "\nPolicy:\n" + formatS3Policy(e.PolicyDocument)

	return out
}

func (gui *Gui) renderVPCTransit(vpc *aws.VPC) tasks.TaskFunc {
	return gui.vpcTab(vpc, func(ctx context.Context, vpcID string) (string, error) {
		attachments, err := gui.Client.ListTGWAttachments(ctx)
		if err != nil {
			return "", err
		}
		gateways, err := gui.Client.ListTransitGateways(ctx)
		if err != nil {
			return "", err
		}
		return formatVPCTransit(vpcID, attachments, gateways), nil
	})
}

func formatVPCSubnets(subnets []aws.Subnet) string {
	if len(subnets) == 0 {
		return "no subnets in this vpc\n"
	}

	rows := make([][]string, len(subnets))
	for i, s := range subnets {
		rows[i] = []string{
			presentation.StatusCell(s.State, presentation.StatusStyleIcon),
			orDash(s.Name),
			s.ID,
			s.CIDR,
			s.AZ,
			vpcSubnetReach(s),
			fmt.Sprintf("%d free", s.AvailableIPs),
		}
	}

	table, err := utils.RenderTable(rows)
	if err != nil {
		return err.Error()
	}
	return table + "\n"
}

// vpcSubnetReach separates two things that get conflated: a public subnet routes to an internet gateway, which is not the same as handing new instances a public IP.
func vpcSubnetReach(s aws.Subnet) string {
	reach := "private"
	if s.Public {
		reach = "public"
	}
	if s.MapPublicIPOnLaunch {
		reach += " +autoip"
	}
	return reach
}

func formatVPCRouteTables(tables []aws.RouteTable) string {
	if len(tables) == 0 {
		return "no route tables in this vpc\n"
	}

	var b strings.Builder
	for i, t := range tables {
		if i > 0 {
			b.WriteString("\n")
		}

		heading := t.ID
		if t.Name != "" {
			heading = t.Name + "  " + t.ID
		}
		if t.Main {
			heading += "  (main)"
		}
		if len(t.SubnetIDs) > 0 {
			heading += "  → " + strings.Join(t.SubnetIDs, ", ")
		}
		b.WriteString(heading + "\n")

		if len(t.Routes) == 0 {
			b.WriteString("  no routes\n")
			continue
		}

		rows := make([][]string, len(t.Routes))
		for j, r := range t.Routes {
			rows[j] = []string{"  " + orDash(r.Destination), orDash(r.Target), r.State, r.Origin}
		}
		table, err := utils.RenderTable(rows)
		if err != nil {
			return err.Error()
		}
		b.WriteString(table + "\n")
	}

	return b.String()
}

func formatVPCGateways(internet []aws.InternetGateway, nat []aws.NATGateway) string {
	var b strings.Builder

	b.WriteString("Internet gateways:\n")
	if len(internet) == 0 {
		b.WriteString("  none\n")
	} else {
		for _, g := range internet {
			b.WriteString(fmt.Sprintf("  %s  %s  %s\n", g.ID, orDash(g.Name), g.State))
		}
	}

	b.WriteString("\nNAT gateways:\n")
	if len(nat) == 0 {
		b.WriteString("  none\n")
		return b.String()
	}

	rows := make([][]string, len(nat))
	for i, g := range nat {
		rows[i] = []string{
			"  " + presentation.StatusCell(g.State, presentation.StatusStyleIcon),
			orDash(g.Name),
			g.ID,
			g.ConnectivityType,
			g.SubnetID,
			orDash(g.PublicIP),
			orDash(g.PrivateIP),
		}
	}
	table, err := utils.RenderTable(rows)
	if err != nil {
		return err.Error()
	}
	b.WriteString(table + "\n")

	for _, g := range nat {
		if g.FailureMessage != "" {
			b.WriteString(fmt.Sprintf("\n%s: %s\n", g.ID, g.FailureMessage))
		}
	}

	return b.String()
}

// formatVPCTransit answers whether this VPC reaches a transit gateway at all, which is the first thing to establish when it cannot talk to a peer network.
func formatVPCTransit(vpcID string, attachments []aws.TGWAttachment, gateways []aws.TransitGateway) string {
	byID := make(map[string]aws.TransitGateway, len(gateways))
	for _, g := range gateways {
		byID[g.ID] = g
	}

	var mine []aws.TGWAttachment
	for _, a := range attachments {
		if a.ResourceID == vpcID {
			mine = append(mine, a)
		}
	}

	if len(mine) == 0 {
		return "this vpc has no transit gateway attachment\n"
	}

	var b strings.Builder
	for i, a := range mine {
		if i > 0 {
			b.WriteString("\n")
		}

		fields := map[string]string{
			"Attachment":        a.ID,
			"Transit gateway":   a.TransitGatewayID,
			"State":             a.State,
			"Association":       orDash(a.RouteTableID),
			"Association state": orDash(a.AssociationState),
			"Created":           formatSecretsTime(a.CreatedAt),
		}
		if a.Name != "" {
			fields["Name"] = a.Name
		}
		// A transit gateway shared in from another account is the usual reason an attachment exists while routing still fails.
		if a.TGWOwnerID != "" && a.TGWOwnerID != a.ResourceOwnerID {
			fields["Owned by"] = a.TGWOwnerID + " (shared from another account)"
		}

		if gateway, known := byID[a.TransitGatewayID]; known {
			fields["Gateway state"] = gateway.State
			if gateway.AmazonSideASN != 0 {
				fields["Amazon-side ASN"] = fmt.Sprintf("%d", gateway.AmazonSideASN)
			}
			if gateway.Description != "" {
				fields["Description"] = gateway.Description
			}
		} else {
			fields["Gateway state"] = "not visible from this account"
		}

		b.WriteString(utils.FormatMap(0, fields))
	}

	return b.String()
}

func formatVPCList(title string, values []string) string {
	if len(values) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n" + title + ":\n")
	for _, value := range values {
		b.WriteString("  " + value + "\n")
	}

	return b.String()
}

func formatVPCTags(tags []aws.Tag) string {
	values := make([]string, len(tags))
	for i, tag := range tags {
		values[i] = tag.Key + "=" + tag.Value
	}
	return formatVPCList("Tags", values)
}

func formatEnabled(on bool) string {
	if on {
		return "enabled"
	}
	return "disabled"
}

func formatYesNo(yes bool) string {
	if yes {
		return "yes"
	}
	return "no"
}
