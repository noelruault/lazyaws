package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/presentation"
	"github.com/noelruault/lazyaws/ui/tasks"
	"github.com/noelruault/lazyaws/ui/utils"
)

// eksLogTypeOrder keeps disabled control-plane log types visible.
var eksLogTypeOrder = []string{"api", "audit", "authenticator", "controllerManager", "scheduler"}

func formatEKSLogTypes(enabledTypes []string) string {
	on := make(map[string]bool, len(enabledTypes))
	for _, t := range enabledTypes {
		on[t] = true
	}

	out := "Control plane logging:\n"
	for _, t := range eksLogTypeOrder {
		status := "disabled"
		if on[t] {
			status = "enabled"
		}
		out += fmt.Sprintf("  %s: %s\n", t, status)
	}
	return out
}

func formatEKSNetworking(details *aws.EKSClusterDetails) string {
	out := ""
	if details.VpcId != "" {
		out += fmt.Sprintf("VPC: %s\n", details.VpcId)
	}
	if len(details.SubnetIds) > 0 {
		out += fmt.Sprintf("Subnets: %s\n", strings.Join(details.SubnetIds, ", "))
	}
	if len(details.SecurityGroupIds) > 0 {
		out += fmt.Sprintf("Security Groups: %s\n", strings.Join(details.SecurityGroupIds, ", "))
	}

	access := "private only"
	switch {
	case details.EndpointPublicAccess && details.EndpointPrivateAccess:
		access = "public + private"
	case details.EndpointPublicAccess:
		access = "public only"
	}
	out += fmt.Sprintf("Endpoint access: %s\n", access)
	if details.EndpointPublicAccess && len(details.PublicAccessCidrs) > 0 {
		out += fmt.Sprintf("Allowed CIDRs: %s\n", strings.Join(details.PublicAccessCidrs, ", "))
	}
	return out
}

func eksContainerInsightsURL(region, clusterName string) string {
	return fmt.Sprintf("https://%s.console.aws.amazon.com/cloudwatch/home?region=%s#container-insights:performance/EKS:Cluster/%s", region, region, clusterName)
}

func (gui *Gui) getEKSPanel() *panels.SideListPanel[*aws.EKSCluster] {
	return &panels.SideListPanel[*aws.EKSCluster]{
		ContextState: &panels.ContextState[*aws.EKSCluster]{
			GetMainTabs: func() []panels.MainTab[*aws.EKSCluster] {
				return []panels.MainTab[*aws.EKSCluster]{
					staticOverviewTab(gui, gui.eksClusterOverview),
					{Key: "config", Title: "Config", Render: gui.renderEKSConfig},
					{Key: "nodegroups", Title: "Node groups", Render: gui.renderEKSNodeGroups},
					{Key: "addons", Title: "Addons", Render: gui.renderEKSAddons},
					{Key: "access", Title: "Access", Render: gui.renderEKSAccess},
				}
			},
			GetItemContextCacheKey: func(c *aws.EKSCluster) string {
				return "eks-" + c.Name
			},
		},

		ListPanel: panels.ListPanel[*aws.EKSCluster]{
			List: panels.NewFilteredList[*aws.EKSCluster](),
			View: gui.Views.EKS,
		},
		NoItemsMessage: "no EKS clusters",
		Gui:            gui.intoInterface(),

		Sort: func(a, b *aws.EKSCluster) bool {
			return a.Name < b.Name
		},
		GetTableCellsFit: func(c *aws.EKSCluster) []utils.Cell {
			return presentation.GetEKSClusterDisplayCells(c)
		},
		Weights: func(*aws.EKSCluster) []int { return presentation.EKSClusterWeights() },
	}
}

func (gui *Gui) loadEKSList() error {
	if gui.Client == nil {
		return nil
	}

	gen := gui.Gen

	return gui.WithWaitingStatus("loading eks", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		clusters, err := gui.Client.ListEKSClusters(ctx)
		if err != nil {
			return err
		}
		if gen != gui.Gen {
			return nil
		}

		rows := make([]*aws.EKSCluster, len(clusters))
		for i := range clusters {
			rows[i] = &clusters[i]
		}
		gui.Panels.EKS.SetItemsKeepSelection(rows, eksSelectionKey)
		return gui.Panels.EKS.RerenderList()
	})
}

// eksSelectionKey identifies a cluster across reloads; cluster names are unique per region.
func eksSelectionKey(cluster *aws.EKSCluster) string { return cluster.Name }

// eksClusterOverview consolidates the Config, Node groups and Addons tabs, reading the cluster's own fields off the list row.
// The tab renders once per selection rather than on a ticker: ListNodeGroups and ListAddons each describe every item they list, so the pane's cost grows with the cluster, and a control plane's version, networking and addon set are not per-tick facts.
func (gui *Gui) eksClusterOverview(ctx context.Context, cluster *aws.EKSCluster, width int) string {
	if gui.Client == nil {
		return overviewUnavailable("cluster")
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	return presentation.FormatEKSClusterOverview(cluster, gui.Client.GetEKSClusterOverview(fetchCtx, cluster.Name), width)
}

func (gui *Gui) renderEKSConfig(cluster *aws.EKSCluster) tasks.TaskFunc {
	name := cluster.Name
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		details, _ := gui.Client.GetEKSClusterDetails(fetchCtx, name)
		if gen != gui.Gen {
			return
		}

		out := fmt.Sprintf("Cluster: %s\n", name)
		out += fmt.Sprintf("Version: %s\n", cluster.Version)
		out += fmt.Sprintf("Status: %s\n", cluster.Status)
		out += fmt.Sprintf("Endpoint: %s\n", cluster.Endpoint)
		out += fmt.Sprintf("Created: %s\n", cluster.CreatedAt)
		out += fmt.Sprintf("Nodes: %d\n", cluster.NodeCount)

		if details != nil {
			out += fmt.Sprintf("\nRole: %s\n", details.RoleArn)
			out += fmt.Sprintf("Region: %s\n", details.Region)

			if details.PlatformVersion != "" {
				out += fmt.Sprintf("Platform: %s\n", details.PlatformVersion)
			}

			out += "\n" + formatEKSNetworking(details)
			out += "\n" + formatEKSLogTypes(details.EnabledLogTypes)
		}

		out += fmt.Sprintf("\nContainer Insights: %s\n", eksContainerInsightsURL(cluster.Region, name))

		// Insights are best-effort so the rest of the cluster configuration still renders.
		insights, _ := gui.Client.ListInsights(fetchCtx, name)
		if len(insights) > 0 {
			out += "\nInsights:\n"
			for _, ins := range insights {
				out += fmt.Sprintf("  [%s] %s (%s)\n", ins.Category, ins.Name, ins.Status)
				if ins.Description != "" {
					out += fmt.Sprintf("    %s\n", ins.Description)
				}
			}
		}

		gui.RenderStringMain(out)
	}})
}

func (gui *Gui) renderEKSNodeGroups(cluster *aws.EKSCluster) tasks.TaskFunc {
	name := cluster.Name
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		nodeGroups, _ := gui.Client.ListNodeGroups(fetchCtx, name)
		if gen != gui.Gen {
			return
		}

		out := fmt.Sprintf("Node Groups for %s:\n", name)
		if len(nodeGroups) == 0 {
			out += "none\n"
			gui.RenderStringMain(out)
			return
		}

		for _, ng := range nodeGroups {
			out += fmt.Sprintf("\n%s (%s)\n", ng.Name, ng.Status)
			out += fmt.Sprintf("  Type: %s\n", ng.AmiType)
			out += fmt.Sprintf("  Desired: %d, Min: %d, Max: %d\n", ng.DesiredSize, ng.MinSize, ng.MaxSize)
			out += fmt.Sprintf("  Version: %s\n", ng.Version)
			out += fmt.Sprintf("  Created: %s\n", ng.CreatedAt)
		}

		gui.RenderStringMain(out)
	}})
}

func (gui *Gui) renderEKSAddons(cluster *aws.EKSCluster) tasks.TaskFunc {
	name := cluster.Name
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		addons, _ := gui.Client.ListAddons(fetchCtx, name)
		if gen != gui.Gen {
			return
		}

		out := fmt.Sprintf("Add-ons for %s:\n", name)
		if len(addons) == 0 {
			out += "none\n"
			gui.RenderStringMain(out)
			return
		}

		for _, addon := range addons {
			out += fmt.Sprintf("\n%s\n", addon.Name)
			out += fmt.Sprintf("  Version: %s\n", addon.Version)
			out += fmt.Sprintf("  Status: %s\n", addon.Status)
			if addon.Health != "" {
				out += fmt.Sprintf("  Health: %s\n", addon.Health)
			}
			out += fmt.Sprintf("  Created: %s\n", addon.CreatedAt)
		}

		gui.RenderStringMain(out)
	}})
}

func (gui *Gui) renderEKSAccess(cluster *aws.EKSCluster) tasks.TaskFunc {
	name := cluster.Name
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		entries, _ := gui.Client.ListAccessEntries(fetchCtx, name)
		podIds, _ := gui.Client.ListPodIdentityAssociations(fetchCtx, name)
		if gen != gui.Gen {
			return
		}

		out := fmt.Sprintf("Access Entries for %s:\n", name)
		if len(entries) == 0 {
			out += "none\n"
		} else {
			for _, entry := range entries {
				out += fmt.Sprintf("  %s\n", entry.PrincipalArn)
				out += fmt.Sprintf("    Type: %s\n", entry.Type)
				if entry.CreatedAt != "" {
					out += fmt.Sprintf("    Created: %s\n", entry.CreatedAt)
				}
			}
		}

		out += fmt.Sprintf("\nPod Identity Associations for %s:\n", name)
		if len(podIds) == 0 {
			out += "none\n"
		} else {
			for _, assoc := range podIds {
				out += fmt.Sprintf("  %s/%s\n", assoc.ServiceAccountNS, assoc.ServiceAccountName)
				out += fmt.Sprintf("    ARN: %s\n", assoc.AssociationArn)
			}
		}

		gui.RenderStringMain(out)
	}})
}
