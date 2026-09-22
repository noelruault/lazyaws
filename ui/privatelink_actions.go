package ui

import (
	"context"
	"fmt"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/resources"
)

// PrivateLinkActions are the service's own actions. Accepting and rejecting live on the connection rows instead, because they take one endpoint and the service does not name one.
func (gui *Gui) PrivateLinkActions() []resources.Action {
	service, err := gui.Panels.PrivateLink.GetSelectedItem()
	if err != nil {
		return nil
	}

	return []resources.Action{
		{
			Name: "Show console URL",
			Run: func(context.Context, string) error {
				return gui.showPopup(service.ID, endpointServiceConsoleURL(gui.Client.Region, service.ID))
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

// endpointConnectionMenu is the affordance the main panel's actions key opens on a connection row.
// It goes through runAction rather than building menu items directly, so accept and reject pass the read-only gate and the confirmation prompts every other mutating action passes.
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
				if err := gui.Client.AcceptVPCEndpointConnections(ctx, connection.ServiceID, []string{connection.EndpointID}); err != nil {
					return err
				}
				return gui.afterEndpointConnectionChange(connection.ServiceID)
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

// rejectConnectionAction grades the prompt by what the rejection costs: a waiting request has no traffic to lose, while an established connection is carrying some, and cutting it is felt on the consumer's side immediately.
func (gui *Gui) rejectConnectionAction(connection aws.VPCEndpointConnection) resources.Action {
	action := resources.Action{
		Name:         "Reject connection",
		Mutates:      true,
		Confirm:      resources.ConfirmSimple,
		Confirmation: fmt.Sprintf("Reject %s from account %s?", connection.EndpointID, orDash(connection.Owner)),
		Run: func(ctx context.Context, _ string) error {
			if err := gui.Client.RejectVPCEndpointConnections(ctx, connection.ServiceID, []string{connection.EndpointID}); err != nil {
				return err
			}
			return gui.afterEndpointConnectionChange(connection.ServiceID)
		},
	}

	if !connection.Pending() {
		action.Confirm = resources.ConfirmDangerous
		action.Token = connection.EndpointID
	}

	return action
}

// afterEndpointConnectionChange puts the new state on screen rather than leaving it to the refresh tick, because accept and reject take effect at once and a row still reading pendingAcceptance invites a second attempt.
func (gui *Gui) afterEndpointConnectionChange(serviceID string) error {
	gui.overviewCache.forget(serviceID)

	if reload, ok := gui.panelReloads[privateLinkReloader]; ok {
		go func() { _ = reload() }()
	}
	gui.rerenderCurrentMainTab()

	return nil
}

// showPopup reports a value the terminal cannot copy for us, matching how the console-URL actions elsewhere answer.
// Run executes off the UI thread, so the popup is queued rather than created here.
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
