package ui

import (
	"context"
	"fmt"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/resources"
)

func (gui *Gui) EKSActions() []resources.Action {
	cluster, err := gui.Panels.EKS.GetSelectedItem()
	if err != nil {
		return nil
	}

	return []resources.Action{{
		Name:    "Upgrade cluster version",
		Mutates: true,
		Prompt:  fmt.Sprintf("Target Kubernetes version for %s (current: %s)", cluster.Name, cluster.Version),
		// Cluster-wide upgrades require the target version before confirmation.
		Confirm:      resources.ConfirmSimple,
		Confirmation: eksUpgradeQuestion(cluster),
		Run: func(ctx context.Context, newVersion string) error {
			if newVersion == "" || newVersion == cluster.Version {
				return nil
			}
			return gui.Client.UpgradeClusterVersion(ctx, cluster.Name, newVersion)
		},
	}}
}

// eksUpgradeQuestion includes the current version so confirmation is informed.
func eksUpgradeQuestion(cluster *aws.EKSCluster) string {
	return fmt.Sprintf("Upgrade %s from %s to the version you entered? Node groups must match or the upgrade is blocked. This cannot be undone.", cluster.Name, cluster.Version)
}
