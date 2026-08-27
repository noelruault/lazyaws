package aws

import (
	"context"
	"errors"
)

// The EKSOverview.Errs keys, one per fetch. Each is a loader the Config, Node groups and Addons tabs already call, so the pane costs no request those three tabs do not.
const (
	SectionCluster    = "cluster"
	SectionNodeGroups = "node-groups"
	SectionAddons     = "addons"
)

// EKSOverview aggregates what the Config, Node groups and Addons tabs each fetch separately.
// The cluster's version, status, endpoint and node count come off the list row; only the describe-only fields and the two lists need a call.
type EKSOverview struct {
	Details    *EKSClusterDetails
	NodeGroups []EKSNodeGroup
	Addons     []EKSAddon

	Errs map[string]error
}

// Err reports the fetch error for a section, if that section failed.
func (o *EKSOverview) Err(section string) error {
	return o.Errs[section]
}

// GetEKSClusterOverview fetches the cluster's describe and its two lists concurrently and always returns an overview, never an error.
// A section that failed is reported through Errs so the sections that succeeded still render: one denied call degrades one block instead of blanking the pane.
func (c *Client) GetEKSClusterOverview(ctx context.Context, clusterName string) *EKSOverview {
	overview := &EKSOverview{Errs: map[string]error{}}
	sections := newSectionFetcher(overview.Errs)

	sections.fetch(SectionCluster, c.eksSection(func() (err error) {
		overview.Details, err = c.GetEKSClusterDetails(ctx, clusterName)
		return err
	}))
	sections.fetch(SectionNodeGroups, c.eksSection(func() (err error) {
		overview.NodeGroups, err = c.ListNodeGroups(ctx, clusterName)
		return err
	}))
	sections.fetch(SectionAddons, c.eksSection(func() (err error) {
		overview.Addons, err = c.ListAddons(ctx, clusterName)
		return err
	}))

	sections.wait()

	return overview
}

// eksSection runs a fetch behind the nil-client check none of the EKS list paths carries, the same guard the instance and VPC overviews put inside their fan-outs.
// Guarding inside the fan-out rather than ahead of it is what keeps the failure per section, and what makes the guard reachable from a test at all: an early return would leave sections.wait and every section key unreached.
func (c *Client) eksSection(run func() error) func() error {
	return func() error {
		if c.EKS == nil {
			return errors.New("EKS client not initialized")
		}

		return run()
	}
}
