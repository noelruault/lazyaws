package ui

import (
	"context"
	"fmt"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/resources"
	"github.com/noelruault/lazyaws/ui/types"
)

// VPCActions stays read-only: deleting an endpoint or turning off its private DNS silently breaks every client that resolves the service through it, and neither is recoverable from this panel.
func (gui *Gui) VPCActions() []resources.Action {
	endpoint, err := gui.Panels.VPC.GetSelectedItem()
	if err != nil {
		return nil
	}

	return []resources.Action{{
		Name: "Show console URL",
		Run: func(context.Context, string) error {
			url := vpcEndpointConsoleURL(gui.Client.Region, endpoint.ID)
			// A popup avoids a clipboard dependency, matching how the presigned-URL action reports its result.
			gui.g.Update(func(*gocui.Gui) error {
				return gui.createConfirmationPanel(endpoint.ID, url, func(*gocui.Gui, *gocui.View) error { return nil }, nil)
			})
			return nil
		},
	}}
}

func vpcEndpointConsoleURL(region, endpointID string) string {
	if region == "" {
		return "https://console.aws.amazon.com/vpcconsole/home#Endpoints:"
	}
	return fmt.Sprintf("https://%s.console.aws.amazon.com/vpcconsole/home?region=%s#EndpointDetails:vpcEndpointId=%s", region, region, endpointID)
}

// vpcEndpointMenu is the affordance the main panel's actions key opens on an endpoint row.
// It stays read-only for the same reason the panel does: every mutation here breaks traffic that is already flowing.
func (gui *Gui) vpcEndpointMenu(endpoint aws.VPCEndpoint) error {
	url := vpcEndpointConsoleURL(gui.Client.Region, endpoint.ID)
	items := []*types.MenuItem{
		{
			Label: "Show console URL",
			OnPress: func() error {
				return gui.createConfirmationPanel(endpoint.ID, url, func(*gocui.Gui, *gocui.View) error { return nil }, nil)
			},
		},
		{
			Label: "Show service name",
			OnPress: func() error {
				return gui.createConfirmationPanel(endpoint.ID, endpoint.ServiceName, func(*gocui.Gui, *gocui.View) error { return nil }, nil)
			},
		},
	}

	return gui.Menu(CreateMenuOptions{Title: "Endpoint: " + endpoint.ShortService(), Items: items})
}
