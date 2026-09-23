package ui

import (
	"context"
	"fmt"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/resources"
)

// PrivateLinkActions changes nothing: accepting and rejecting live on the connection rows, because they take one endpoint and the service does not name one.
func (gui *Gui) PrivateLinkActions() []resources.Action {
	service, err := gui.Panels.PrivateLink.GetSelectedItem()
	if err != nil {
		return nil
	}

	return []resources.Action{
		{
			Name: "Show console URL",
			Run: func(context.Context, string) error {
				return gui.showPopup(service.ID, endpointServiceConsoleURL(gui.awsClient().Region, service.ID))
			},
		},
		{
			// The service name, not the id, is what a consumer needs to create their endpoint, and it is too long to read off the list row.
			Name: "Show service name",
			Run: func(context.Context, string) error {
				return gui.showPopup(service.ID, service.Name)
			},
		},
	}
}

// endpointConnectionMenu goes through openActionsMenu rather than building menu items directly, so accept and reject pass the read-only and confirmation gates runAction enforces.
func (gui *Gui) endpointConnectionMenu(connection aws.VPCEndpointConnection) error {
	return gui.openActionsMenu("Endpoint: "+connection.EndpointID, gui.endpointConnectionActions(connection))
}

func (gui *Gui) endpointConnectionActions(connection aws.VPCEndpointConnection) []resources.Action {
	actions := make([]resources.Action, 0, 3)

	// Accept only appears on a request that is waiting: EC2 rejects the call for a connection in any other state, and an action that can only fail is worse than an action that is not there.
	if connection.Pending() {
		actions = append(actions, resources.Action{
			Name:         "Accept connection",
			Mutates:      true,
			Confirm:      resources.ConfirmSimple,
			Confirmation: fmt.Sprintf("Accept %s from account %s?", connection.EndpointID, orDash(connection.Owner)),
			Run: func(ctx context.Context, _ string) error {
				if err := gui.awsClient().AcceptVPCEndpointConnections(ctx, connection.ServiceID, []string{connection.EndpointID}); err != nil {
					return err
				}
				return gui.afterEndpointConnectionChange(connection, aws.VPCEndpointStatePending)
			},
		})
	}

	actions = append(actions, gui.rejectConnectionAction(connection), resources.Action{
		Name: "Show console URL",
		Run: func(context.Context, string) error {
			return gui.showPopup(connection.EndpointID, vpcEndpointConsoleURL(connection.Region, connection.EndpointID))
		},
	})

	return actions
}

// rejectConnectionAction grades the prompt by what the rejection costs: a waiting request has no traffic to lose, an established connection is carrying some.
func (gui *Gui) rejectConnectionAction(connection aws.VPCEndpointConnection) resources.Action {
	action := resources.Action{
		Name:         "Reject connection",
		Mutates:      true,
		Confirm:      resources.ConfirmSimple,
		Confirmation: fmt.Sprintf("Reject %s from account %s?", connection.EndpointID, orDash(connection.Owner)),
		Run: func(ctx context.Context, _ string) error {
			if err := gui.awsClient().RejectVPCEndpointConnections(ctx, connection.ServiceID, []string{connection.EndpointID}); err != nil {
				return err
			}
			return gui.afterEndpointConnectionChange(connection, aws.VPCEndpointStateRejected)
		},
	}

	if !connection.Pending() {
		action.Confirm = resources.ConfirmDangerous
		action.Token = connection.EndpointID
	}

	return action
}

// afterEndpointConnectionChange puts the new state on screen itself, because the reload below is single-flighted and a dropped one leaves a row reading pendingAcceptance, which invites a second attempt.
func (gui *Gui) afterEndpointConnectionChange(connection aws.VPCEndpointConnection, state string) error {
	gui.overviewCache.forget(connection.ServiceID)

	gui.queueUpdate(func() error {
		gui.setEndpointConnectionState(connection.ServiceID, connection.EndpointID, state)
		gui.rerenderCurrentMainTab()

		return nil
	})

	// Best effort confirmation: the guard may drop it, and the rows no longer depend on it landing.
	if reload, ok := gui.panelReloads[privateLinkReloader]; ok {
		go func() { _ = reload() }()
	}

	return nil
}

// showPopup queues the popup rather than creating it here, because an action's Run executes off the UI thread.
func (gui *Gui) showPopup(title, body string) error {
	gui.g.Update(func(*gocui.Gui) error {
		return gui.createConfirmationPanel(title, body, func(*gocui.Gui, *gocui.View) error { return nil }, nil)
	})

	return nil
}

func endpointServiceConsoleURL(region, serviceID string) string {
	if region == "" {
		return "https://console.aws.amazon.com/vpcconsole/home#EndpointServices:"
	}
	return fmt.Sprintf("https://%s.console.aws.amazon.com/vpcconsole/home?region=%s#EndpointServiceDetails:serviceId=%s", region, region, serviceID)
}
