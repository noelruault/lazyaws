package ui

import (
	"strings"
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/resources"
)

func mustFindAction(t *testing.T, actions []resources.Action, name string) resources.Action {
	t.Helper()
	action, ok := findAction(actions, name)
	if !ok {
		t.Fatalf("no %q in %v", name, actionNames(actions))
	}
	return action
}

// EC2 refuses an accept on a connection that is not waiting, so an Accept offered there could only ever fail.
func TestAcceptIsOfferedOnlyOnAWaitingConnection(t *testing.T) {
	gui := newTestGui(t)

	waiting := gui.endpointConnectionActions(aws.VPCEndpointConnection{
		ServiceID:  "vpce-svc-0123456789abcdef0",
		EndpointID: "vpce-0123456789abcdef0",
		Owner:      "111122223333",
		State:      aws.VPCEndpointStatePendingAcceptance,
	})
	if got := actionNames(waiting); !strings.Contains(strings.Join(got, ","), "Accept connection") {
		t.Errorf("a waiting connection offers %v, want an Accept", got)
	}

	live := gui.endpointConnectionActions(aws.VPCEndpointConnection{
		ServiceID:  "vpce-svc-0123456789abcdef0",
		EndpointID: "vpce-0123456789abcdef0",
		State:      "available",
	})
	if got := actionNames(live); strings.Contains(strings.Join(got, ","), "Accept connection") {
		t.Errorf("an established connection offers %v, want no Accept", got)
	}
}

// Rejecting a request that is waiting costs the consumer a retry; rejecting a connection that is carrying traffic cuts it, so the two cannot share one prompt.
func TestRejectIsGradedByWhatItCosts(t *testing.T) {
	gui := newTestGui(t)

	waiting := mustFindAction(t, gui.endpointConnectionActions(aws.VPCEndpointConnection{
		EndpointID: "vpce-0123456789abcdef0",
		State:      aws.VPCEndpointStatePendingAcceptance,
	}), "Reject connection")
	if waiting.Confirm != resources.ConfirmSimple {
		t.Errorf("reject on a waiting request confirms %v, want a simple prompt", waiting.Confirm)
	}

	live := mustFindAction(t, gui.endpointConnectionActions(aws.VPCEndpointConnection{
		EndpointID: "vpce-0123456789abcdef0",
		State:      "available",
	}), "Reject connection")
	if live.Confirm != resources.ConfirmDangerous {
		t.Errorf("reject on an established connection confirms %v, want the typed token", live.Confirm)
	}
	if live.Token != "vpce-0123456789abcdef0" {
		t.Errorf("reject token = %q, want the endpoint id the row shows", live.Token)
	}
	if err := live.Valid(); err != nil {
		t.Errorf("the dangerous reject is not a valid action: %v", err)
	}
}

// An action that reaches AWS and is not marked Mutates would run in a read-only session, which is the one promise this app makes on the front page.
func TestEveryConnectionActionThatChangesAWSIsMarked(t *testing.T) {
	gui := newTestGui(t)

	for _, connection := range []aws.VPCEndpointConnection{
		{EndpointID: "vpce-1", State: aws.VPCEndpointStatePendingAcceptance},
		{EndpointID: "vpce-1", State: "available"},
	} {
		for _, action := range gui.endpointConnectionActions(connection) {
			changes := strings.HasPrefix(action.Name, "Accept") || strings.HasPrefix(action.Name, "Reject")
			if changes != action.Mutates {
				t.Errorf("%q in state %q has Mutates=%v", action.Name, connection.State, action.Mutates)
			}
		}
	}
}

// The service's own menu is read-only on purpose: accept and reject name one endpoint, and the service does not.
func TestTheServiceMenuChangesNothing(t *testing.T) {
	gui := newTestGui(t)
	gui.Panels.PrivateLink.SetItems([]*aws.VPCEndpointService{{ID: "vpce-svc-0123456789abcdef0", NameTag: "payments"}})

	for _, action := range gui.PrivateLinkActions() {
		if action.Mutates {
			t.Errorf("%q on the service menu is marked mutating", action.Name)
		}
	}
}

// A read-only session must be offered nothing it would then refuse, which is what dropMutatingItems is for.
func TestReadOnlyLeavesOnlyTheSafeConnectionActions(t *testing.T) {
	gui, _ := newReadOnlyHeadlessGui(t)

	items := gui.dropMutatingItems(gui.actionMenuItems(gui.endpointConnectionActions(aws.VPCEndpointConnection{
		ServiceID:  "vpce-svc-0123456789abcdef0",
		EndpointID: "vpce-0123456789abcdef0",
		State:      aws.VPCEndpointStatePendingAcceptance,
	})))

	if len(items) != 1 || items[0].Label != "Show console URL" {
		labels := make([]string, len(items))
		for i, item := range items {
			labels[i] = item.Label
		}
		t.Errorf("read-only connection menu = %v, want the console URL alone", labels)
	}
}

func TestEndpointServiceConsoleURL(t *testing.T) {
	got := endpointServiceConsoleURL("eu-west-1", "vpce-svc-0123456789abcdef0")
	for _, want := range []string{"eu-west-1.console.aws.amazon.com", "region=eu-west-1", "serviceId=vpce-svc-0123456789abcdef0"} {
		if !strings.Contains(got, want) {
			t.Errorf("console URL %q omits %q", got, want)
		}
	}

	// A client built before a region was resolved must still produce a link that opens somewhere useful.
	if regionless := endpointServiceConsoleURL("", "vpce-svc-1"); !strings.HasPrefix(regionless, "https://console.aws.amazon.com/") {
		t.Errorf("console URL without a region = %q", regionless)
	}
}
