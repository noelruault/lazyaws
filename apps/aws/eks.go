package aws

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
)

type EKSCluster struct {
	Name      string
	Version   string
	Status    string
	Endpoint  string
	Region    string
	CreatedAt string
	NodeCount int
	Arn       string
}

type EKSClusterDetails struct {
	Name                  string
	Version               string
	Status                string
	Endpoint              string
	Region                string
	CreatedAt             string
	Arn                   string
	RoleArn               string
	CertificateAuthority  string
	VpcId                 string
	SubnetIds             []string
	SecurityGroupIds      []string
	EndpointPublicAccess  bool
	EndpointPrivateAccess bool
	PublicAccessCidrs     []string
	EnabledLogTypes       []string
	PlatformVersion       string
	Tags                  map[string]string
}

type EKSNodeGroup struct {
	Name          string
	Status        string
	InstanceTypes []string
	DesiredSize   int32
	MinSize       int32
	MaxSize       int32
	AmiType       string
	CreatedAt     string
	Version       string
	NodeRole      string
}

type EKSAddon struct {
	Name               string
	Version            string
	Status             string
	Health             string
	CreatedAt          string
	ModifiedAt         string
	ServiceAccountRole string
}

type AccessEntry struct {
	PrincipalArn   string
	AccessEntryArn string
	CreatedAt      string
	ModifiedAt     string
	Type           string // USER, ROLE, or FEDERATION_ACCOUNT
}

type FargateProfile struct {
	Name             string
	ClusterName      string
	Status           string
	CreatedAt        string
	Selectors        []string // namespace summary
	PodExecutionRole string
}

type ClusterInsight struct {
	ID                 string
	Name               string
	Status             string
	Category           string
	Description        string
	LastRefreshTime    string
	LastTransitionTime string
}

type PodIdentityAssociation struct {
	ServiceAccountName string
	ServiceAccountNS   string
	AssociationArn     string
	AssociationId      string
}

func (c *Client) ListEKSClusters(ctx context.Context) ([]EKSCluster, error) {
	input := &eks.ListClustersInput{}
	var clusterNames []string
	for {
		result, err := c.EKS.ListClusters(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to list EKS clusters: %w", err)
		}
		clusterNames = append(clusterNames, result.Clusters...)
		if result.NextToken == nil {
			break
		}
		input.NextToken = result.NextToken
	}

	var clusters []EKSCluster
	for _, clusterName := range clusterNames {
		details, err := c.GetEKSClusterDetails(ctx, clusterName)
		if err != nil {
			clusters = append(clusters, EKSCluster{
				Name:   clusterName,
				Status: "unknown",
				Region: c.Region,
			})
			continue
		}

		nodeCount, _ := c.countNodeGroupNodes(ctx, clusterName)

		clusters = append(clusters, EKSCluster{
			Name:      details.Name,
			Version:   details.Version,
			Status:    details.Status,
			Endpoint:  details.Endpoint,
			Region:    c.Region,
			CreatedAt: details.CreatedAt,
			NodeCount: nodeCount,
			Arn:       details.Arn,
		})
	}

	return clusters, nil
}

func (c *Client) GetEKSClusterDetails(ctx context.Context, clusterName string) (*EKSClusterDetails, error) {
	input := &eks.DescribeClusterInput{
		Name: &clusterName,
	}

	result, err := c.EKS.DescribeCluster(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe cluster: %w", err)
	}

	cluster := result.Cluster
	details := &EKSClusterDetails{
		Name:    getString(cluster.Name),
		Version: getString(cluster.Version),
		Status:  string(cluster.Status),
		Region:  c.Region,
		Arn:     getString(cluster.Arn),
	}

	if cluster.Endpoint != nil {
		details.Endpoint = *cluster.Endpoint
	}

	if cluster.CreatedAt != nil {
		details.CreatedAt = cluster.CreatedAt.Format("2006-01-02 15:04:05")
	}

	if cluster.RoleArn != nil {
		details.RoleArn = *cluster.RoleArn
	}

	if cluster.CertificateAuthority != nil && cluster.CertificateAuthority.Data != nil {
		details.CertificateAuthority = *cluster.CertificateAuthority.Data
	}

	if cluster.ResourcesVpcConfig != nil {
		if cluster.ResourcesVpcConfig.VpcId != nil {
			details.VpcId = *cluster.ResourcesVpcConfig.VpcId
		}
		details.SubnetIds = cluster.ResourcesVpcConfig.SubnetIds
		details.SecurityGroupIds = cluster.ResourcesVpcConfig.SecurityGroupIds
		details.EndpointPublicAccess = cluster.ResourcesVpcConfig.EndpointPublicAccess
		details.EndpointPrivateAccess = cluster.ResourcesVpcConfig.EndpointPrivateAccess
		details.PublicAccessCidrs = cluster.ResourcesVpcConfig.PublicAccessCidrs
	}

	if cluster.Logging != nil && cluster.Logging.ClusterLogging != nil {
		for _, logSetup := range cluster.Logging.ClusterLogging {
			if logSetup.Enabled != nil && *logSetup.Enabled {
				for _, logType := range logSetup.Types {
					details.EnabledLogTypes = append(details.EnabledLogTypes, string(logType))
				}
			}
		}
	}

	if cluster.PlatformVersion != nil {
		details.PlatformVersion = *cluster.PlatformVersion
	}

	if cluster.Tags != nil {
		details.Tags = cluster.Tags
	}

	return details, nil
}

func (c *Client) ListNodeGroups(ctx context.Context, clusterName string) ([]EKSNodeGroup, error) {
	input := &eks.ListNodegroupsInput{
		ClusterName: &clusterName,
	}

	result, err := c.EKS.ListNodegroups(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list node groups: %w", err)
	}

	var nodeGroups []EKSNodeGroup
	for _, ngName := range result.Nodegroups {
		ng, err := c.GetNodeGroupDetails(ctx, clusterName, ngName)
		if err != nil {
			nodeGroups = append(nodeGroups, EKSNodeGroup{
				Name:   ngName,
				Status: "unknown",
			})
			continue
		}
		nodeGroups = append(nodeGroups, *ng)
	}

	return nodeGroups, nil
}

func (c *Client) GetNodeGroupDetails(ctx context.Context, clusterName, nodeGroupName string) (*EKSNodeGroup, error) {
	input := &eks.DescribeNodegroupInput{
		ClusterName:   &clusterName,
		NodegroupName: &nodeGroupName,
	}

	result, err := c.EKS.DescribeNodegroup(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe node group: %w", err)
	}

	ng := result.Nodegroup
	details := &EKSNodeGroup{
		Name:   getString(ng.NodegroupName),
		Status: string(ng.Status),
	}

	if ng.InstanceTypes != nil {
		details.InstanceTypes = ng.InstanceTypes
	}

	if ng.ScalingConfig != nil {
		if ng.ScalingConfig.DesiredSize != nil {
			details.DesiredSize = *ng.ScalingConfig.DesiredSize
		}
		if ng.ScalingConfig.MinSize != nil {
			details.MinSize = *ng.ScalingConfig.MinSize
		}
		if ng.ScalingConfig.MaxSize != nil {
			details.MaxSize = *ng.ScalingConfig.MaxSize
		}
	}

	if ng.AmiType != "" {
		details.AmiType = string(ng.AmiType)
	}

	if ng.CreatedAt != nil {
		details.CreatedAt = ng.CreatedAt.Format("2006-01-02 15:04:05")
	}

	if ng.Version != nil {
		details.Version = *ng.Version
	}

	if ng.NodeRole != nil {
		details.NodeRole = *ng.NodeRole
	}

	return details, nil
}

func (c *Client) ListAddons(ctx context.Context, clusterName string) ([]EKSAddon, error) {
	input := &eks.ListAddonsInput{
		ClusterName: &clusterName,
	}

	result, err := c.EKS.ListAddons(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list add-ons: %w", err)
	}

	var addons []EKSAddon
	for _, addonName := range result.Addons {
		addon, err := c.GetAddonDetails(ctx, clusterName, addonName)
		if err != nil {
			addons = append(addons, EKSAddon{
				Name:   addonName,
				Status: "unknown",
			})
			continue
		}
		addons = append(addons, *addon)
	}

	return addons, nil
}

func (c *Client) GetAddonDetails(ctx context.Context, clusterName, addonName string) (*EKSAddon, error) {
	input := &eks.DescribeAddonInput{
		ClusterName: &clusterName,
		AddonName:   &addonName,
	}

	result, err := c.EKS.DescribeAddon(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe add-on: %w", err)
	}

	addon := result.Addon
	details := &EKSAddon{
		Name:    getString(addon.AddonName),
		Version: getString(addon.AddonVersion),
		Status:  string(addon.Status),
	}

	if addon.Health != nil {
		for _, issue := range addon.Health.Issues {
			if issue.Code != "" {
				details.Health = string(issue.Code)
				break
			}
		}
		if details.Health == "" {
			details.Health = "Healthy"
		}
	}

	if addon.CreatedAt != nil {
		details.CreatedAt = addon.CreatedAt.Format("2006-01-02 15:04:05")
	}

	if addon.ModifiedAt != nil {
		details.ModifiedAt = addon.ModifiedAt.Format("2006-01-02 15:04:05")
	}

	if addon.ServiceAccountRoleArn != nil {
		details.ServiceAccountRole = *addon.ServiceAccountRoleArn
	}

	return details, nil
}

func (c *Client) UpdateKubeconfig(ctx context.Context, clusterName string) error {
	// Rewrites the operator's ~/.kube/config, which is a change to their machine rather than to AWS, and still not one a read-only session should make.
	if err := requireWrites("rewriting ~/.kube/config for " + clusterName); err != nil {
		return err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	kubeconfigPath := filepath.Join(homeDir, ".kube", "config")

	cmd := exec.CommandContext(ctx, "aws", "eks", "update-kubeconfig",
		"--name", clusterName,
		"--region", c.Region,
		"--kubeconfig", kubeconfigPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update kubeconfig: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// UpgradeClusterVersion requires callers to gate the hard-to-reverse operation.
func (c *Client) UpgradeClusterVersion(ctx context.Context, clusterName, newVersion string) error {
	if c.EKS == nil {
		return fmt.Errorf("EKS client not initialized")
	}
	if clusterName == "" {
		return fmt.Errorf("cluster name is required")
	}
	if newVersion == "" {
		return fmt.Errorf("target version is required")
	}

	_, err := c.EKS.UpdateClusterVersion(ctx, &eks.UpdateClusterVersionInput{
		Name:    &clusterName,
		Version: &newVersion,
	})
	if err != nil {
		return fmt.Errorf("failed to upgrade cluster %s to version %s: %w", clusterName, newVersion, err)
	}
	return nil
}

// GetClusterLogs only returns the CloudWatch log-group hint after verifying the type is enabled; it does not retrieve events.
func (c *Client) GetClusterLogs(ctx context.Context, clusterName string, logType types.LogType) ([]string, error) {
	details, err := c.GetEKSClusterDetails(ctx, clusterName)
	if err != nil {
		return nil, err
	}

	enabled := false
	for _, lt := range details.EnabledLogTypes {
		if lt == string(logType) {
			enabled = true
			break
		}
	}

	if !enabled {
		return nil, fmt.Errorf("log type %s is not enabled for cluster %s", logType, clusterName)
	}

	logGroupName := fmt.Sprintf("/aws/eks/%s/cluster", clusterName)
	return []string{
		fmt.Sprintf("Logs are available in CloudWatch Logs group: %s", logGroupName),
		fmt.Sprintf("Log type: %s", logType),
	}, nil
}

func (c *Client) countNodeGroupNodes(ctx context.Context, clusterName string) (int, error) {
	nodeGroups, err := c.ListNodeGroups(ctx, clusterName)
	if err != nil {
		return 0, err
	}

	totalNodes := 0
	for _, ng := range nodeGroups {
		totalNodes += int(ng.DesiredSize)
	}

	return totalNodes, nil
}

func (c *Client) ListAccessEntries(ctx context.Context, clusterName string) ([]AccessEntry, error) {
	input := &eks.ListAccessEntriesInput{
		ClusterName: &clusterName,
	}

	var entries []AccessEntry
	for {
		result, err := c.EKS.ListAccessEntries(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to list access entries: %w", err)
		}

		for _, arn := range result.AccessEntries {
			detail, err := c.DescribeAccessEntry(ctx, clusterName, arn)
			if err != nil {
				entries = append(entries, AccessEntry{
					PrincipalArn: arn,
					Type:         "unknown",
				})
				continue
			}
			entries = append(entries, *detail)
		}

		if result.NextToken == nil {
			break
		}
		input.NextToken = result.NextToken
	}

	return entries, nil
}

func (c *Client) DescribeAccessEntry(ctx context.Context, clusterName, principalArn string) (*AccessEntry, error) {
	input := &eks.DescribeAccessEntryInput{
		ClusterName:  &clusterName,
		PrincipalArn: &principalArn,
	}

	result, err := c.EKS.DescribeAccessEntry(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe access entry: %w", err)
	}

	entry := &AccessEntry{
		PrincipalArn:   getString(result.AccessEntry.PrincipalArn),
		AccessEntryArn: getString(result.AccessEntry.AccessEntryArn),
	}

	if result.AccessEntry.Type != nil {
		entry.Type = *result.AccessEntry.Type
	}

	if result.AccessEntry.CreatedAt != nil {
		entry.CreatedAt = result.AccessEntry.CreatedAt.Format("2006-01-02 15:04:05")
	}

	if result.AccessEntry.ModifiedAt != nil {
		entry.ModifiedAt = result.AccessEntry.ModifiedAt.Format("2006-01-02 15:04:05")
	}

	return entry, nil
}

func (c *Client) ListFargateProfiles(ctx context.Context, clusterName string) ([]FargateProfile, error) {
	input := &eks.ListFargateProfilesInput{
		ClusterName: &clusterName,
	}

	var profiles []FargateProfile
	for {
		result, err := c.EKS.ListFargateProfiles(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to list Fargate profiles: %w", err)
		}

		for _, profileName := range result.FargateProfileNames {
			detail, err := c.DescribeFargateProfile(ctx, clusterName, profileName)
			if err != nil {
				profiles = append(profiles, FargateProfile{
					Name:        profileName,
					ClusterName: clusterName,
					Status:      "unknown",
				})
				continue
			}
			profiles = append(profiles, *detail)
		}

		if result.NextToken == nil {
			break
		}
		input.NextToken = result.NextToken
	}

	return profiles, nil
}

func (c *Client) DescribeFargateProfile(ctx context.Context, clusterName, profileName string) (*FargateProfile, error) {
	input := &eks.DescribeFargateProfileInput{
		ClusterName:        &clusterName,
		FargateProfileName: &profileName,
	}

	result, err := c.EKS.DescribeFargateProfile(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe Fargate profile: %w", err)
	}

	profile := &FargateProfile{
		Name:             getString(result.FargateProfile.FargateProfileName),
		ClusterName:      clusterName,
		Status:           string(result.FargateProfile.Status),
		PodExecutionRole: getString(result.FargateProfile.PodExecutionRoleArn),
	}

	if result.FargateProfile.CreatedAt != nil {
		profile.CreatedAt = result.FargateProfile.CreatedAt.Format("2006-01-02 15:04:05")
	}

	var selectors []string
	if result.FargateProfile.Selectors != nil {
		for _, sel := range result.FargateProfile.Selectors {
			ns := "all"
			if sel.Namespace != nil {
				ns = *sel.Namespace
			}
			selectors = append(selectors, ns)
		}
	}
	profile.Selectors = selectors

	return profile, nil
}

func (c *Client) ListInsights(ctx context.Context, clusterName string) ([]ClusterInsight, error) {
	input := &eks.ListInsightsInput{
		ClusterName: &clusterName,
	}

	var insights []ClusterInsight
	for {
		result, err := c.EKS.ListInsights(ctx, input)
		if err != nil {
			return nil, nil
		}

		for _, insight := range result.Insights {
			if insight.Id != nil {
				ins := ClusterInsight{
					ID:       *insight.Id,
					Name:     getString(insight.Name),
					Category: string(insight.Category),
				}
				if insight.Description != nil {
					ins.Description = *insight.Description
				}
				if insight.InsightStatus != nil {
					ins.Status = string(insight.InsightStatus.Status)
				}
				if insight.LastRefreshTime != nil {
					ins.LastRefreshTime = insight.LastRefreshTime.Format("2006-01-02 15:04:05")
				}
				if insight.LastTransitionTime != nil {
					ins.LastTransitionTime = insight.LastTransitionTime.Format("2006-01-02 15:04:05")
				}
				insights = append(insights, ins)
			}
		}

		if result.NextToken == nil {
			break
		}
		input.NextToken = result.NextToken
	}

	return insights, nil
}

func (c *Client) ListPodIdentityAssociations(ctx context.Context, clusterName string) ([]PodIdentityAssociation, error) {
	input := &eks.ListPodIdentityAssociationsInput{
		ClusterName: &clusterName,
	}

	var associations []PodIdentityAssociation
	for {
		result, err := c.EKS.ListPodIdentityAssociations(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to list Pod Identity associations: %w", err)
		}

		for _, assoc := range result.Associations {
			association := PodIdentityAssociation{
				ServiceAccountName: getString(assoc.ServiceAccount),
				ServiceAccountNS:   getString(assoc.Namespace),
				AssociationArn:     getString(assoc.AssociationArn),
				AssociationId:      getString(assoc.AssociationId),
			}
			associations = append(associations, association)
		}

		if result.NextToken == nil {
			break
		}
		input.NextToken = result.NextToken
	}

	return associations, nil
}
