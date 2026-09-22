package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

func TestEndpointConnectionRowCells(t *testing.T) {
	created := time.Date(2026, 9, 17, 8, 30, 0, 0, time.UTC)
	cells := endpointConnectionRowCells(&aws.VPCEndpointConnection{
		EndpointID: "vpce-0123456789abcdef0",
		State:      aws.VPCEndpointStatePendingAcceptance,
		Owner:      "111122223333",
		Region:     "eu-west-1",
		CreatedAt:  &created,
	})

	row := utils.Decolorise(strings.Join(cells, " "))
	for _, want := range []string{"vpce-0123456789abcdef0", "111122223333", "eu-west-1"} {
		if !strings.Contains(row, want) {
			t.Errorf("row %q omits %q", row, want)
		}
	}

	// A connection that answered without an owner or a region must not leave the columns blank and unlabelled.
	bare := utils.Decolorise(strings.Join(endpointConnectionRowCells(&aws.VPCEndpointConnection{EndpointID: "vpce-1"}), " "))
	if !strings.Contains(bare, "-") {
		t.Errorf("row for a bare connection = %q, want dashes for the empty fields", bare)
	}
}

func TestFormatEndpointConnectionDetailShowsWhatTheRowCannot(t *testing.T) {
	created := time.Date(2026, 9, 17, 8, 30, 0, 0, time.UTC)
	out := utils.Decolorise(formatEndpointConnectionDetail(&aws.VPCEndpointConnection{
		EndpointID:       "vpce-0123456789abcdef0",
		ServiceID:        "vpce-svc-0123456789abcdef0",
		ConnectionID:     "vpce-con-01234567890abcdef",
		State:            aws.VPCEndpointStatePendingAcceptance,
		Owner:            "111122223333",
		Region:           "eu-west-1",
		IPAddressType:    "ipv4",
		CreatedAt:        &created,
		DNSNames:         []string{"vpce-0ec31.eu-west-1.vpce.amazonaws.com"},
		LoadBalancerARNs: []string{"arn:aws:elasticloadbalancing:eu-west-1:111122223333:loadbalancer/net/api-nlb/abc123"},
		Tags:             []aws.Tag{{Key: "env", Value: "staging"}},
	}))

	for _, want := range []string{
		"vpce-svc-0123456789abcdef0",
		"vpce-con-01234567890abcdef",
		"ipv4",
		"vpce-0ec31.eu-west-1.vpce.amazonaws.com",
		"api-nlb",
		"env=staging",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("detail omits %q:\n%s", want, out)
		}
	}
}

// The panel exists to answer "is anything waiting", so the services that are waiting have to be the ones at the top of it.
func TestPrivateLinkListPutsWaitingServicesFirst(t *testing.T) {
	gui := newTestGui(t)

	gui.Panels.PrivateLink.SetItems([]*aws.VPCEndpointService{
		{ID: "vpce-svc-a", NameTag: "alpha"},
		{ID: "vpce-svc-c", NameTag: "charlie", Pending: 1},
		{ID: "vpce-svc-b", NameTag: "bravo"},
		{ID: "vpce-svc-d", NameTag: "delta", Pending: 4},
	})

	got := make([]string, 0, 4)
	for _, service := range gui.Panels.PrivateLink.List.GetItems() {
		got = append(got, service.Label())
	}

	want := []string{"delta", "charlie", "alpha", "bravo"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v (most pending first, then by label)", got, want)
		}
	}
}
