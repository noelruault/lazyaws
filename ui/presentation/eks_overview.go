package presentation

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// errClusterNotDescribed stands in when the describe recorded no error and still produced no cluster, so a section reading it says which fact is missing instead of dereferencing nil.
var errClusterNotDescribed = errors.New("cluster not described")

// FormatEKSClusterOverview lays a cluster out for the Overview tab: what the control plane is on the left, and what is running under it on the right.
// Everything comes off the list row or the Config, Node groups and Addons tabs' own loaders, so the pane costs no call those tabs do not already make.
func FormatEKSClusterOverview(c *aws.EKSCluster, o *aws.EKSOverview, width int) string {
	// Cut to the pane: the header spans the full width rather than a column, so Columns never measures it, and a long cluster name beside the version and the region runs off the edge unmarked.
	header := truncateBlock(ResourceHeader("EKS cluster", c.Name, Badge(c.Status), "", eksVersionLabel(c.Version), c.Region, eksCreated(c)), width)

	column := ColumnWidth(width, overviewGap)
	left := joinBlocks(eksConfigBlock(c, o), eksNetworkingBlock(o), eksLoggingBlock(o), eksTagsBlock(o, column))
	right := joinBlocks(eksNodeGroupsBlock(c, o, column), eksAddonsBlock(o, column))

	return header + "\n\n" + Columns(width, overviewGap, left, right)
}

// eksVersionLabel prefixes the Kubernetes version the way kubectl and the EKS console do, so it is not mistaken for the platform version beside it.
func eksVersionLabel(version string) string {
	if version == "" {
		return ""
	}

	return "v" + version
}

func eksCreated(c *aws.EKSCluster) string {
	if c.CreatedAt == "" {
		return ""
	}

	return "created " + c.CreatedAt
}

// eksDetailsErr reports why the describe cannot be read from, covering both a failed fetch and an answer that never arrived.
// The three blocks below all read the same describe, so each one asks this rather than repeating a nil check the row's own fields do not need.
func eksDetailsErr(o *aws.EKSOverview) error {
	if err := o.Err(aws.SectionCluster); err != nil {
		return err
	}
	if o.Details == nil {
		return errClusterNotDescribed
	}

	return nil
}

// eksConfigBlock reads the list row, not the describe, so a denied DescribeCluster still leaves the cluster's version, status and endpoint on the pane.
func eksConfigBlock(c *aws.EKSCluster, o *aws.EKSOverview) string {
	rows := []kv{
		{"Version", orNone(c.Version)},
		{"Status", orNone(c.Status)},
		{"Endpoint", orNone(c.Endpoint)},
		{"Region", orNone(c.Region)},
		{"Created", orNone(c.CreatedAt)},
		{"Nodes", fmt.Sprintf("%d desired", c.NodeCount)},
		{"Platform", eksPlatformVersion(o)},
		{"ARN", orNone(c.Arn)},
	}

	return SectionTitle("Configuration") + "\n" + kvBlock(rows)
}

// eksPlatformVersion is the one Configuration row the list row cannot answer: the EKS platform version is a describe-only field, and it is what an AWS support case asks for.
func eksPlatformVersion(o *aws.EKSOverview) string {
	if err := eksDetailsErr(o); err != nil {
		return fieldOr(err, "")
	}

	return orNone(o.Details.PlatformVersion)
}

func eksNetworkingBlock(o *aws.EKSOverview) string {
	if err := eksDetailsErr(o); err != nil {
		return sectionUnavailable("Networking", err)
	}
	d := o.Details

	rows := []kv{
		{"VPC", orNone(d.VpcId)},
		{"Subnets", eksIDsLine(d.SubnetIds)},
		{"Security groups", eksIDsLine(d.SecurityGroupIds)},
		{"Endpoint access", eksEndpointAccess(d)},
		{"Allowed CIDRs", eksPublicAccessCIDRs(d)},
	}

	return SectionTitle("Networking") + "\n" + kvBlock(rows)
}

// eksIDsLine counts before listing, because a cluster spans a subnet per AZ and the count is the fact read at a glance while the ids are what a rule gets written against.
func eksIDsLine(ids []string) string {
	if len(ids) == 0 {
		return "none"
	}

	sorted := slices.Sorted(slices.Values(ids))

	return fmt.Sprintf("%d · %s", len(sorted), strings.Join(sorted, ", "))
}

// eksEndpointAccess colours a control plane reachable from the internet, which is the posture question asked of an EKS cluster and the one a private cluster is built to answer.
// A cluster with neither flag is not a fourth posture: EKS requires at least one, so it is reported as read rather than guessed at.
func eksEndpointAccess(d *aws.EKSClusterDetails) string {
	switch {
	case d.EndpointPublicAccess && d.EndpointPrivateAccess:
		return utils.ColoredString("public + private", color.FgYellow)
	case d.EndpointPublicAccess:
		return utils.ColoredString("public only", color.FgYellow)
	case d.EndpointPrivateAccess:
		return "private only"
	default:
		return "none reported"
	}
}

// eksPublicAccessCIDRs is only meaningful while the public endpoint is on: the list restricts that endpoint and nothing else, so rendering it beside a disabled endpoint would read as an exposure that is switched off.
func eksPublicAccessCIDRs(d *aws.EKSClusterDetails) string {
	if !d.EndpointPublicAccess {
		return "n/a, public access off"
	}
	if len(d.PublicAccessCidrs) == 0 {
		return "none"
	}

	cidrs := slices.Sorted(slices.Values(d.PublicAccessCidrs))
	line := strings.Join(cidrs, ", ")
	// Open to the whole internet is the one value worth colouring: it is also the AWS default, so it is reached by not deciding rather than by deciding.
	if slices.Contains(cidrs, "0.0.0.0/0") {
		return utils.ColoredString(line, color.FgYellow)
	}

	return line
}

// eksLogTypes is every control-plane log type EKS can emit, so a disabled one is reported as off rather than omitted.
// An audit trail nobody switched on is invisible exactly when it is needed, and the absence of a line does not say that.
var eksLogTypes = []string{"api", "audit", "authenticator", "controllerManager", "scheduler"}

func eksLoggingBlock(o *aws.EKSOverview) string {
	if err := eksDetailsErr(o); err != nil {
		return sectionUnavailable("Control plane logging", err)
	}

	enabled := make(map[string]bool, len(o.Details.EnabledLogTypes))
	for _, logType := range o.Details.EnabledLogTypes {
		enabled[logType] = true
	}

	on := make([]string, 0, len(eksLogTypes))
	off := make([]string, 0, len(eksLogTypes))
	for _, logType := range eksLogTypes {
		if enabled[logType] {
			on = append(on, logType)
			continue
		}
		off = append(off, logType)
	}

	rows := []kv{
		{"Enabled", orNoneList(on)},
		{"Disabled", eksDisabledLogTypes(off)},
	}

	return SectionTitle("Control plane logging") + "\n" + kvBlock(rows)
}

// eksDisabledLogTypes leaves the fully-logged cluster uncoloured and marks every other case, because a partial log configuration reads as configured until the missing types are named.
func eksDisabledLogTypes(off []string) string {
	if len(off) == 0 {
		return "none"
	}

	return utils.ColoredString(strings.Join(off, ", "), color.FgYellow)
}

func eksTagsBlock(o *aws.EKSOverview, width int) string {
	title := SectionTitle("Tags")
	if err := eksDetailsErr(o); err != nil {
		return sectionUnavailable("Tags", err)
	}
	if len(o.Details.Tags) == 0 {
		return title + "\nnone"
	}

	return title + "\n" + tagsBodyFrom(width, o.Details.Tags)
}

func eksNodeGroupsBlock(c *aws.EKSCluster, o *aws.EKSOverview, width int) string {
	title := SectionTitle("Node groups")
	if err := o.Err(aws.SectionNodeGroups); err != nil {
		return sectionUnavailable("Node groups", err)
	}
	if len(o.NodeGroups) == 0 {
		// A cluster with no managed node groups is not a broken cluster: it may run on Fargate or on self-managed nodes, and the pane must not imply otherwise.
		return title + "\nnone (Fargate or self-managed nodes)"
	}

	// Sorted here rather than trusted from the caller: ListNodegroups returns them in whatever order the API answers, and an unsorted table reshuffles between renders of the same cluster.
	groups := slices.Clone(o.NodeGroups)
	slices.SortStableFunc(groups, func(a, b aws.EKSNodeGroup) int { return strings.Compare(a.Name, b.Name) })

	desired := int32(0)
	byType := map[string]int{}
	for _, group := range groups {
		desired += group.DesiredSize
		for _, instanceType := range group.InstanceTypes {
			byType[instanceType]++
		}
	}

	rows := make([][]utils.Cell, len(groups))
	for i, group := range groups {
		rows[i] = []utils.Cell{
			{Text: group.Name},
			BadgeCell(group.Status),
			{Text: eksScalingLabel(group)},
			eksNodeGroupVersionCell(group, c.Version),
		}
	}

	// The name is the only column without a natural width, so it takes the slack and the rest are content-sized.
	table, _ := utils.RenderTableFit(rows, width, []int{1, 0, 0, 0})

	lines := []string{
		title,
		fmt.Sprintf("%s · %d nodes desired", pluralize(len(groups), "node group"), desired),
		"types: " + eksTypesLine(byType),
		table,
	}

	return strings.Join(lines, "\n")
}

// eksTypesLine keeps the per-group instance types off the table, where a fifth column would starve the name at half a pane, and reports them for the cluster instead.
func eksTypesLine(byType map[string]int) string {
	if len(byType) == 0 {
		return "none reported"
	}

	return countsByKey(byType)
}

// eksScalingLabel renders the desired size with the bounds it can move between, "3 (1-6)", because a group already at its maximum cannot absorb a scale-up and the desired count alone does not say so.
func eksScalingLabel(group aws.EKSNodeGroup) string {
	return fmt.Sprintf("%d (%d-%d)", group.DesiredSize, group.MinSize, group.MaxSize)
}

// eksNodeGroupVersionCell colours a node group whose Kubernetes version trails the control plane, which is the drift that makes a cluster upgrade fail halfway.
// A group reporting no version is "-" rather than drifted: an unknown version is not evidence of a mismatch.
func eksNodeGroupVersionCell(group aws.EKSNodeGroup, clusterVersion string) utils.Cell {
	if group.Version == "" {
		return utils.Cell{Text: "-", Color: color.Faint}
	}
	if clusterVersion != "" && group.Version != clusterVersion {
		return utils.Cell{Text: "v" + group.Version + " ⚠", Color: color.FgYellow}
	}

	return utils.Cell{Text: "v" + group.Version}
}

func eksAddonsBlock(o *aws.EKSOverview, width int) string {
	title := SectionTitle("Addons")
	if err := o.Err(aws.SectionAddons); err != nil {
		return sectionUnavailable("Addons", err)
	}
	if len(o.Addons) == 0 {
		return title + "\nnone"
	}

	// Sorted for the same reason as the node groups: ListAddons imposes no order, and the table must not reshuffle between renders.
	addons := slices.Clone(o.Addons)
	slices.SortStableFunc(addons, func(a, b aws.EKSAddon) int { return strings.Compare(a.Name, b.Name) })

	rows := make([][]utils.Cell, len(addons))
	for i, addon := range addons {
		rows[i] = []utils.Cell{
			{Text: addon.Name},
			BadgeCell(addon.Status),
			{Text: orNone(addon.Version), Color: color.Faint},
			eksAddonHealthCell(addon.Health),
		}
	}

	table, _ := utils.RenderTableFit(rows, width, []int{1, 0, 0, 0})

	return title + "\n" + pluralize(len(addons), "addon") + "\n" + table
}

// eksAddonHealthCell separates a healthy addon from one AWS reported no health for at all: the describe omits the health block entirely on some addons, and reading that as healthy hides the one state worth acting on.
func eksAddonHealthCell(health string) utils.Cell {
	switch health {
	case "":
		return utils.Cell{Text: "-", Color: color.Faint}
	case "Healthy":
		return utils.Cell{Text: "healthy", Color: color.FgGreen}
	default:
		return utils.Cell{Text: health, Color: color.FgRed}
	}
}
