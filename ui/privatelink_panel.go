package ui

import (
	"context"
	"time"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/presentation"
	"github.com/noelruault/lazyaws/ui/tasks"
	"github.com/noelruault/lazyaws/ui/utils"
)

const privateLinkFetchTimeout = 20 * time.Second

// privateLinkReloader is the key the panel's loader is registered under, named once so the actions that reload the list after accepting a connection cannot drift from it.
const privateLinkReloader = "privatelink"

func (gui *Gui) getPrivateLinkPanel() *panels.SideListPanel[*aws.VPCEndpointService] {
	return &panels.SideListPanel[*aws.VPCEndpointService]{
		ContextState: &panels.ContextState[*aws.VPCEndpointService]{
			GetMainTabs: func() []panels.MainTab[*aws.VPCEndpointService] {
				return []panels.MainTab[*aws.VPCEndpointService]{
					staticOverviewTab(gui, endpointServiceCacheKey, gui.privateLinkOverview),
					{
						Key:    "connections",
						Title:  "Connections",
						Render: gui.renderEndpointConnections,
						Rows:   func(*aws.VPCEndpointService) *panels.MainRows { return gui.endpointConnectionRows() },
					},
				}
			},
			GetItemContextCacheKey: endpointServiceCacheKey,
		},

		ListPanel: panels.ListPanel[*aws.VPCEndpointService]{
			List: panels.NewFilteredList[*aws.VPCEndpointService](),
			View: gui.Views.PrivateLink,
		},
		NoItemsMessage: "no endpoint services",
		Gui:            gui.intoInterface(),

		// Services with connections waiting sort first, and by how many: this panel exists to answer "is anything queued", so the answer sits at the top of it rather than in alphabetical order.
		Sort: func(a, b *aws.VPCEndpointService) bool {
			if a.Pending != b.Pending {
				return a.Pending > b.Pending
			}
			return a.Label() < b.Label()
		},
		GetTableCellsFit: func(s *aws.VPCEndpointService) []utils.Cell {
			return presentation.GetVPCEndpointServiceDisplayCells(s)
		},
		Weights: func(*aws.VPCEndpointService) []int { return presentation.PrivateLinkWeights() },
		// The service id, not the service name: the id is what accept-vpc-endpoint-connections and every other provider-side call take.
		CopyValue: func(s *aws.VPCEndpointService) string { return s.ID },
	}
}

func endpointServiceCacheKey(s *aws.VPCEndpointService) string { return s.ID }

func (gui *Gui) loadPrivateLinkList() error {
	if gui.Client == nil {
		return nil
	}

	gen := gui.Generation()

	return gui.WhileWaiting("loading endpoint services", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), privateLinkFetchTimeout)
		defer cancel()

		services, err := gui.Client.ListVPCEndpointServices(ctx)
		if err != nil {
			return err
		}
		if gen != gui.Generation() {
			return nil
		}

		rows := make([]*aws.VPCEndpointService, len(services))
		for i := range services {
			rows[i] = &services[i]
		}
		swapPanelItems(gui, gui.Panels.PrivateLink, rows, endpointServiceCacheKey)

		return nil
	})
}

// privateLinkOverview reads the connections the service's own list call cannot carry, so the pane can report them by state.
func (gui *Gui) privateLinkOverview(ctx context.Context, service *aws.VPCEndpointService, width int) string {
	if gui.Client == nil {
		return overviewUnavailable("PrivateLink")
	}

	fetchCtx, cancel := context.WithTimeout(ctx, privateLinkFetchTimeout)
	defer cancel()

	connections, err := gui.Client.ListVPCEndpointConnections(fetchCtx, service.ID)

	return presentation.FormatVPCEndpointServiceOverview(service, connections, err, width)
}

// endpointConnectionsState keeps the last fetch so the main panel can address its rows, and remembers whether one record is open, because the whole record is already in hand and reopening it costs no call.
type endpointConnectionsState struct {
	serviceID   string
	connections []aws.VPCEndpointConnection
	showing     bool
	detail      int
}

func (gui *Gui) renderEndpointConnections(service *aws.VPCEndpointService) tasks.TaskFunc {
	serviceID := service.ID

	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Generation()
		fetchCtx, cancel := context.WithTimeout(ctx, privateLinkFetchTimeout)
		defer cancel()

		connections, err := gui.Client.ListVPCEndpointConnections(fetchCtx, serviceID)
		if gen != gui.Generation() {
			return
		}
		if err != nil {
			gui.RenderStringMain("error: " + err.Error())
			return
		}

		// The row handlers read this state on the UI loop, so the fetch hands it over there rather than writing it from the task goroutine.
		gui.Update(func() error {
			// Moving to another service closes whatever record was open, since its index means nothing in the new list.
			if gui.endpointConnections.serviceID != serviceID {
				gui.endpointConnections = endpointConnectionsState{serviceID: serviceID}
			}
			gui.endpointConnections.connections = connections

			gui.RenderStringMain(gui.endpointConnectionsContent())

			return nil
		})
	}})
}

// endpointConnectionsContent renders whichever of the two views is current; both read the same in-memory fetch.
func (gui *Gui) endpointConnectionsContent() string {
	state := gui.endpointConnections
	if state.showing && state.detail >= 0 && state.detail < len(state.connections) {
		return formatEndpointConnectionDetail(&state.connections[state.detail])
	}

	rows := gui.endpointConnectionRows()
	return renderMainRows(rows, gui.mainCursor(rows))
}

func (gui *Gui) rerenderEndpointConnections() error {
	gui.reRenderStringMain(gui.endpointConnectionsContent())
	return nil
}

func (gui *Gui) endpointConnectionRows() *panels.MainRows {
	state := gui.endpointConnections

	// While a record is open there is nothing to walk, so the keys scroll it and Esc returns to the list.
	if state.showing {
		return &panels.MainRows{
			Back: func() error {
				gui.endpointConnections.showing = false
				return gui.rerenderEndpointConnections()
			},
		}
	}

	cells := make([][]string, len(state.connections))
	for i := range state.connections {
		cells[i] = endpointConnectionRowCells(&state.connections[i])
	}

	return &panels.MainRows{
		EmptyMessage: "no connections to this service",
		Cells:        cells,
		Enter: func(i int) error {
			gui.endpointConnections.showing = true
			gui.endpointConnections.detail = i
			return gui.rerenderEndpointConnections()
		},
		Actions: func(i int) error {
			return gui.endpointConnectionMenu(state.connections[i])
		},
	}
}

func endpointConnectionRowCells(c *aws.VPCEndpointConnection) []string {
	return []string{
		presentation.StatusCell(c.State, presentation.StatusStyleIcon),
		c.EndpointID,
		// The owner is the account that asked, which is the fact a request is judged on.
		orDash(c.Owner),
		orDash(c.Region),
		formatSecretsTime(c.CreatedAt),
	}
}

// formatEndpointConnectionDetail shows the fields DescribeVpcEndpointConnections already returned but the row had no width for.
func formatEndpointConnectionDetail(c *aws.VPCEndpointConnection) string {
	fields := map[string]string{
		"Endpoint":   c.EndpointID,
		"Service":    c.ServiceID,
		"State":      c.State,
		"Owner":      orDash(c.Owner),
		"Region":     orDash(c.Region),
		"Created":    formatSecretsTime(c.CreatedAt),
		"IP types":   orDash(c.IPAddressType),
		"Connection": orDash(c.ConnectionID),
	}

	out := utils.FormatMap(0, fields)
	out += formatVPCList("DNS names", c.DNSNames)
	out += formatVPCList("Load balancers", c.LoadBalancerARNs)
	out += formatVPCTags(c.Tags)

	return out
}
