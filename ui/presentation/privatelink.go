package presentation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// PrivateLinkWeights gives the label the slack and sizes the pending count to its content, which is one short cell on the rows that have it and nothing on the rows that do not.
func PrivateLinkWeights() []int {
	return []int{0, 1, 0}
}

// GetVPCEndpointServiceDisplayCells leads with the pending count after the name, because a service with connections waiting is the only reason this list is urgent.
func GetVPCEndpointServiceDisplayCells(s *aws.VPCEndpointService) []utils.Cell {
	return []utils.Cell{
		StatusCellFit(s.State, StatusStyleIcon),
		{Text: s.Label(), Color: color.Bold},
		pendingCell(s.Pending),
	}
}

// pendingCell stays blank at zero: a column of "0 pending" would make the one row that is waiting harder to find.
func pendingCell(pending int) utils.Cell {
	if pending == 0 {
		return utils.Cell{}
	}

	return utils.Cell{Text: fmt.Sprintf("%d pending", pending), Color: color.FgYellow}
}

// FormatVPCEndpointServiceOverview renders connErr rather than swallowing it, because a connections read that failed and a service with no connections look identical once the count is gone.
func FormatVPCEndpointServiceOverview(s *aws.VPCEndpointService, connections []aws.VPCEndpointConnection, connErr error, width int) string {
	header := HeaderWithStats(width,
		ResourceHeader("Endpoint service", s.Label(), "", s.ID, s.Name, acceptanceNote(s)),
		endpointServiceStatCards(s, connections, connErr),
	)

	left := joinBlocks(endpointServiceConfigBlock(s), endpointServiceDNSBlock(s), endpointServiceTagsBlock(s, ColumnWidth(width, overviewGap)))
	right := joinBlocks(endpointConnectionsBlock(connections, connErr), endpointServiceBalancersBlock(s))

	return header + "\n\n" + Columns(width, overviewGap, left, right)
}

// acceptanceNote is in the header because it decides whether anything on this panel needs doing: a service that auto-accepts never queues a request.
func acceptanceNote(s *aws.VPCEndpointService) string {
	if s.AcceptanceRequired {
		return "acceptance required"
	}
	return "auto-accept"
}

func endpointServiceStatCards(s *aws.VPCEndpointService, connections []aws.VPCEndpointConnection, connErr error) []Stat {
	total := utils.Cell{Text: fmt.Sprintf("%d", len(connections))}
	pending := pendingCell(s.Pending)
	if pending.Text == "" {
		pending = utils.Cell{Text: "0"}
	}
	if connErr != nil {
		total = utils.Cell{Text: "unavailable", Color: color.FgRed}
	}

	return []Stat{
		{Label: "State", Value: BadgeCell(s.State)},
		{Label: "Pending", Value: pending},
		{Label: "Connections", Value: total},
		{Label: "Acceptance", Value: utils.Cell{Text: acceptanceNote(s)}},
	}
}

func endpointServiceConfigBlock(s *aws.VPCEndpointService) string {
	rows := []kv{
		{"Service name", orNone(s.Name)},
		{"Acceptance", acceptanceNote(s)},
		{"IP types", orNoneList(s.SupportedIPTypes)},
		{"Payer", orNone(s.PayerResponsibility)},
		{"Zones", orNoneList(s.AvailabilityZones)},
	}
	// Endpoints on a managed service cannot be accepted or rejected through this API, so the row is only worth a line when it is true.
	if s.ManagesEndpoints {
		rows = append(rows, kv{"Managed by", "the service, not this account"})
	}

	return SectionTitle("Configuration") + "\n" + kvBlock(rows)
}

func endpointServiceDNSBlock(s *aws.VPCEndpointService) string {
	rows := []kv{
		{"Private DNS", orNone(s.PrivateDNSName)},
		{"Endpoint DNS", orNoneList(s.BaseDNSNames)},
	}

	return SectionTitle("DNS") + "\n" + kvBlock(rows)
}

func endpointServiceBalancersBlock(s *aws.VPCEndpointService) string {
	title := SectionTitle("Load balancers")
	if len(s.LoadBalancerARNs) == 0 {
		return title + "\nnone"
	}

	names := make([]string, len(s.LoadBalancerARNs))
	for i, arn := range s.LoadBalancerARNs {
		names[i] = loadBalancerName(arn)
	}

	return title + "\n" + strings.Join(names, "\n")
}

// loadBalancerName takes the name off a load balancer ARN, whose last two segments are the name and the id AWS appended to it.
func loadBalancerName(arn string) string {
	parts := strings.Split(arn, "/")
	if len(parts) < 3 {
		return arn
	}

	return parts[len(parts)-2]
}

// endpointConnectionsBlock counts the connections by state rather than listing them: the Connections tab is the list.
func endpointConnectionsBlock(connections []aws.VPCEndpointConnection, err error) string {
	title := SectionTitle("Connections")
	if err != nil {
		return sectionUnavailable("Connections", err)
	}
	if len(connections) == 0 {
		return title + "\nnone"
	}

	counts := map[string]int{}
	for _, connection := range connections {
		counts[connection.State]++
	}

	states := make([]string, 0, len(counts))
	for state := range counts {
		states = append(states, state)
	}
	sort.Strings(states)

	rows := make([]kv, len(states))
	for i, state := range states {
		rows[i] = kv{state, fmt.Sprintf("%d", counts[state])}
	}

	return title + "\n" + kvBlock(rows)
}

func endpointServiceTagsBlock(s *aws.VPCEndpointService, width int) string {
	title := SectionTitle("Tags")
	if len(s.Tags) == 0 {
		return title + "\nnone"
	}

	rows := make([]kv, len(s.Tags))
	for i, tag := range s.Tags {
		rows[i] = kv{tag.Key, tag.Value}
	}

	return title + "\n" + tagChips(width, rows)
}
