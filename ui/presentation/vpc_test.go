package presentation

import (
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

func TestGetVPCDisplayCells(t *testing.T) {
	// The CIDR leads because it is what a VPC gets recognised by when tracing reachability between networks.
	wantCells(t, GetVPCDisplayCells(&aws.VPC{
		ID:    "vpc-0123456789abcdef0",
		Name:  "stage-vpc",
		State: "available",
		CIDR:  "10.0.0.0/16",
	}), []utils.Cell{
		{Text: "▶", Color: color.FgGreen},
		{Text: "10.0.0.0/16", Color: color.Bold},
		{Text: "stage-vpc"},
		{Text: "vpc-0123456789abcdef0", Color: color.Faint},
	})
}

func TestGetVPCDisplayCellsMarksTheDefaultVPC(t *testing.T) {
	got := GetVPCDisplayCells(&aws.VPC{ID: "vpc-1", State: "available", CIDR: "172.31.0.0/16", IsDefault: true})

	if !strings.Contains(got[2].Text, "(default)") {
		t.Errorf("label cell = %q, want the default marker", got[2].Text)
	}
	if !strings.Contains(got[2].Text, "(no name)") {
		t.Errorf("label cell = %q, want the unnamed placeholder kept", got[2].Text)
	}
}

// Every VPC row carries a 21-cell id, so a side panel squeezes this row shape even with a short label.
func TestVPCRowKeepsTheCIDRAndBothIdentifiersInANarrowPanel(t *testing.T) {
	forceColor(t)
	const width = 40

	vpc := &aws.VPC{
		ID:    "vpc-0123456789abcdef0",
		Name:  strings.Repeat("v", 40),
		State: "available",
		CIDR:  "10.0.0.0/16",
	}

	rendered, err := utils.RenderTableFit([][]utils.Cell{GetVPCDisplayCells(vpc)}, width, VPCWeights())
	if err != nil {
		t.Fatalf("RenderTableFit: %v", err)
	}

	plain := utils.Decolorise(rendered)
	if got := runewidth.StringWidth(plain); got > width {
		t.Errorf("row is %d cells wide, want at most %d: %q", got, width, plain)
	}
	// The CIDR is content-sized, so it is the one column that must never be cut: a truncated network address is a different network.
	if !strings.HasPrefix(plain, "▶ 10.0.0.0/16 ") {
		t.Errorf("row = %q, want the icon and the whole CIDR", plain)
	}
	for _, want := range []string{"vvv", "vpc-0"} {
		if !strings.Contains(plain, want) {
			t.Errorf("row = %q, want it to still show %q", plain, want)
		}
	}
}

// A proportional share of a wide panel under-pays a column whose content is a fixed 21 cells, so the id used to be cut at width 60 while the label column sat on idle padding.
func TestVPCRowShowsTheWholeIDOnceThePanelHasRoom(t *testing.T) {
	forceColor(t)

	vpc := &aws.VPC{ID: "vpc-0123456789abcdef0", Name: "stage-vpc", State: "available", CIDR: "10.0.0.0/16"}

	rendered, err := utils.RenderTableFit([][]utils.Cell{GetVPCDisplayCells(vpc)}, 60, VPCWeights())
	if err != nil {
		t.Fatalf("RenderTableFit: %v", err)
	}

	plain := utils.Decolorise(rendered)
	if !strings.Contains(plain, "vpc-0123456789abcdef0") {
		t.Errorf("row = %q, want the whole vpc id at a width that fits it", plain)
	}
	if strings.Contains(plain, "…") {
		t.Errorf("row = %q, want nothing cut at a width everything fits in", plain)
	}
}

func TestVPCWeightsMatchTheRowWidth(t *testing.T) {
	if got, want := len(VPCWeights()), len(GetVPCDisplayCells(&aws.VPC{})); got != want {
		t.Errorf("%d weights for %d cells", got, want)
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
