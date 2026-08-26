package ui

import (
	"strings"
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
)

func TestFormatEKSLogTypesAllDisabled(t *testing.T) {
	out := formatEKSLogTypes(nil)
	for _, lt := range eksLogTypeOrder {
		if !strings.Contains(out, lt+": disabled") {
			t.Errorf("expected %q to show disabled, got:\n%s", lt, out)
		}
	}
}

func TestFormatEKSLogTypesSomeEnabled(t *testing.T) {
	out := formatEKSLogTypes([]string{"api", "audit"})
	if !strings.Contains(out, "api: enabled") || !strings.Contains(out, "audit: enabled") {
		t.Errorf("expected api/audit enabled, got:\n%s", out)
	}
	if !strings.Contains(out, "scheduler: disabled") {
		t.Errorf("expected scheduler disabled, got:\n%s", out)
	}
}

func TestFormatEKSNetworkingPrivateOnly(t *testing.T) {
	details := &aws.EKSClusterDetails{
		VpcId:                 "vpc-123",
		SubnetIds:             []string{"subnet-1", "subnet-2"},
		SecurityGroupIds:      []string{"sg-1"},
		EndpointPrivateAccess: true,
	}
	out := formatEKSNetworking(details)
	if !strings.Contains(out, "VPC: vpc-123") {
		t.Errorf("expected VPC id, got:\n%s", out)
	}
	if !strings.Contains(out, "Subnets: subnet-1, subnet-2") {
		t.Errorf("expected joined subnets, got:\n%s", out)
	}
	if !strings.Contains(out, "Endpoint access: private only") {
		t.Errorf("expected private-only access, got:\n%s", out)
	}
	if strings.Contains(out, "Allowed CIDRs") {
		t.Errorf("expected no CIDR line when public access is off, got:\n%s", out)
	}
}

func TestFormatEKSNetworkingPublicWithCIDRs(t *testing.T) {
	details := &aws.EKSClusterDetails{
		EndpointPublicAccess:  true,
		EndpointPrivateAccess: true,
		PublicAccessCidrs:     []string{"1.2.3.4/32"},
	}
	out := formatEKSNetworking(details)
	if !strings.Contains(out, "Endpoint access: public + private") {
		t.Errorf("expected public + private access, got:\n%s", out)
	}
	if !strings.Contains(out, "Allowed CIDRs: 1.2.3.4/32") {
		t.Errorf("expected CIDR list, got:\n%s", out)
	}
}

func TestEksContainerInsightsURL(t *testing.T) {
	url := eksContainerInsightsURL("eu-west-1", "prod")
	if !strings.Contains(url, "eu-west-1") || !strings.Contains(url, "prod") {
		t.Errorf("expected region and cluster name in URL, got: %s", url)
	}
}

func TestEksUpgradeQuestionNamesTheClusterAndItsVersion(t *testing.T) {
	cluster := &aws.EKSCluster{Name: "prod", Version: "1.29"}
	question := eksUpgradeQuestion(cluster)
	if !strings.Contains(question, "prod") || !strings.Contains(question, "1.29") {
		t.Errorf("expected the cluster name and its current version in the question, got: %s", question)
	}
}

// An empty side panel used to render as a blank box, with its explanation only in the main panel and only while that panel had focus.
func TestRerenderListShowsTheEmptyMessageInTheSideView(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, func() error {
		gui.Panels.EKS.SetItems(nil)
		return gui.Panels.EKS.RerenderList()
	})

	buffer := ask(g, func() string { return gui.Views.EKS.Buffer() })
	if !strings.Contains(buffer, "no EKS clusters") {
		t.Errorf("EKS view = %q, want the panel's empty message", buffer)
	}
}

// A panel with rows must not gain the empty message, and the rows themselves must still be there.
func TestRerenderListDropsTheEmptyMessageOnceRowsArrive(t *testing.T) {
	gui, g := newHeadlessGui(t)

	run(t, g, func() error {
		gui.Panels.EKS.SetItems([]*aws.EKSCluster{{Name: "prod", Status: "ACTIVE"}})
		return gui.Panels.EKS.RerenderList()
	})

	buffer := stripANSIForTest(ask(g, func() string { return gui.Views.EKS.Buffer() }))
	if strings.Contains(buffer, "no EKS clusters") {
		t.Errorf("EKS view = %q, want no empty message when a cluster is listed", buffer)
	}
	if !strings.Contains(buffer, "prod") {
		t.Errorf("EKS view = %q, want the cluster row", buffer)
	}
}
