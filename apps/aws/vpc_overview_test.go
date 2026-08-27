package aws

import (
	"context"
	"strings"
	"testing"
)

// A client with no SDK clients is the one state a test can drive without an interface seam, and it proves the fan-out's contract: every section fails, every failure is reported, and none of them takes the pane down with it.
// It is also what proves the guard is inside the fan-out: the VPC describes dereference c.EC2 directly, so without it this test panics in a goroutine rather than failing.
func TestGetVPCOverviewReportsEverySectionThatFailed(t *testing.T) {
	overview := (&Client{}).GetVPCOverview(context.Background(), "vpc-0abcdef1234567890")

	if overview == nil {
		t.Fatal("GetVPCOverview() = nil, want an overview even when every section failed")
	}

	for _, section := range []string{SectionDNS, SectionSubnets, SectionIGW, SectionNAT, SectionEndpoints} {
		err := overview.Err(section)
		if err == nil {
			t.Errorf("Err(%q) = nil, want the failed fetch to be reported", section)
			continue
		}
		if !strings.Contains(err.Error(), "EC2 client not initialized") {
			t.Errorf("Err(%q) = %v, want the nil-client guard rather than a panic further in", section, err)
		}
	}

	if overview.Subnets != nil || overview.InternetGateways != nil || overview.NATGateways != nil || overview.Endpoints != nil {
		t.Error("a failed section should leave its list nil rather than an empty one the formatter would render as none")
	}
	// Both DNS attributes default to on in AWS, so a failed read must not leave the formatter a false to render as "off".
	if overview.DNSSupport || overview.DNSHostnames {
		t.Error("a failed DNS read reported an attribute as set")
	}
}

// Err answers per section, which is what lets one formatter section render "unavailable" while its neighbours render data.
func TestVPCOverviewErrIsPerSection(t *testing.T) {
	overview := &VPCOverview{Errs: map[string]error{SectionNAT: context.Canceled}}

	if overview.Err(SectionNAT) == nil {
		t.Error("Err() did not report the section that failed")
	}
	if overview.Err(SectionSubnets) != nil {
		t.Error("Err() reported a section that did not fail")
	}
}
