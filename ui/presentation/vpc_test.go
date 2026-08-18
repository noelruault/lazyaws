package presentation

import (
	"strings"
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

func TestGetVPCDisplayStrings(t *testing.T) {
	got := GetVPCDisplayStrings(&aws.VPC{
		ID:    "vpc-0123456789abcdef0",
		Name:  "stage-vpc",
		State: "available",
		CIDR:  "10.0.0.0/16",
	})

	if len(got) != 4 {
		t.Fatalf("got %d cells, want 4", len(got))
	}
	if cell := utils.Decolorise(got[0]); cell != "▶" {
		t.Errorf("status cell = %q, want the running icon", cell)
	}
	// The CIDR leads because it is what a VPC gets recognised by when tracing reachability between networks.
	if got[1] != "10.0.0.0/16" {
		t.Errorf("first text cell = %q, want the CIDR", got[1])
	}
	if got[2] != "stage-vpc" {
		t.Errorf("label cell = %q", got[2])
	}
}

func TestGetVPCDisplayStringsMarksTheDefaultVPC(t *testing.T) {
	got := GetVPCDisplayStrings(&aws.VPC{ID: "vpc-1", State: "available", CIDR: "172.31.0.0/16", IsDefault: true})

	if !strings.Contains(got[2], "(default)") {
		t.Errorf("label cell = %q, want the default marker", got[2])
	}
	if !strings.Contains(got[2], "(no name)") {
		t.Errorf("label cell = %q, want the unnamed placeholder kept", got[2])
	}
}

func TestGetVPCEndpointDisplayStrings(t *testing.T) {
	e := &aws.VPCEndpoint{
		ID:          "vpce-0123456789abcdef0",
		Name:        "secrets-endpoint",
		ServiceName: "com.amazonaws.eu-west-1.secretsmanager",
		Type:        "Interface",
		State:       "Available",
		VpcID:       "vpc-0123456789abcdef0",
	}

	got := GetVPCEndpointDisplayStrings(e)
	if len(got) != 5 {
		t.Fatalf("got %d cells, want 5", len(got))
	}
	if cell := utils.Decolorise(got[0]); cell != "▶" {
		t.Errorf("status cell = %q, want the running icon", cell)
	}

	want := []string{"secrets-endpoint", "vpce-0123456789abcdef0", "Interface", "vpc-0123456789abcdef0"}
	for i, w := range want {
		if got[i+1] != w {
			t.Errorf("cell %d = %q, want %q", i+1, got[i+1], w)
		}
	}
}

// Endpoints are usually untagged, so the fallback label is the common case rather than an edge case.
func TestGetVPCEndpointDisplayStringsFallsBackToTheService(t *testing.T) {
	e := &aws.VPCEndpoint{
		ID:          "vpce-abc",
		ServiceName: "com.amazonaws.eu-west-1.s3",
		Type:        "Gateway",
		State:       "Available",
	}

	if got := GetVPCEndpointDisplayStrings(e)[1]; got != "s3" {
		t.Errorf("label cell = %q, want the shortened service", got)
	}
}
