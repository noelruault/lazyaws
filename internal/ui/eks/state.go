package eks

import (
	"context"

	"github.com/fuziontech/lazyaws/internal/aws"
)

// State contains EKS-specific UI and data state.
type State struct {
	Clusters      []aws.EKSCluster
	Filtered      []aws.EKSCluster
	SelectedIndex int
	Details       *aws.EKSClusterDetails
	NodeGroups    []aws.EKSNodeGroup
	Addons        []aws.EKSAddon
}

// LoadClusters fetches EKS clusters.
func LoadClusters(ctx context.Context, client *aws.Client) ([]aws.EKSCluster, error) {
	if client == nil {
		return nil, nil
	}
	return client.ListEKSClusters(ctx)
}

// LoadClusterDetails fetches details for a specific EKS cluster.
func LoadClusterDetails(ctx context.Context, client *aws.Client, name string) (*aws.EKSClusterDetails, []aws.EKSNodeGroup, []aws.EKSAddon, error) {
	if client == nil || name == "" {
		return nil, nil, nil, nil
	}
	details, err := client.GetEKSClusterDetails(ctx, name)
	if err != nil {
		return nil, nil, nil, err
	}
	nodeGroups, err := client.ListNodeGroups(ctx, name)
	if err != nil {
		return details, nil, nil, err
	}
	addons, err := client.ListAddons(ctx, name)
	if err != nil {
		return details, nodeGroups, nil, err
	}
	return details, nodeGroups, addons, nil
}
