package presentation

import (
	"fmt"
	"strings"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// EKSClusterWeights gives the cluster name all the slack: the badge, the node count and the date are fixed-width, and the name is the only column whose content varies.
func EKSClusterWeights() []int {
	return []int{0, 1, 0, 0}
}

func GetEKSClusterDisplayCells(c *aws.EKSCluster) []utils.Cell {
	return []utils.Cell{
		// A badge rather than the icon alone: EKS reports UPDATING and the client writes "unknown" for a cluster it could not describe, and both collapse to "?" once the word is gone.
		BadgeCell(c.Status),
		{Text: c.Name, Color: color.Bold},
		{Text: fmt.Sprintf("%d nodes", c.NodeCount)},
		{Text: createdDate(c.CreatedAt), Color: color.Faint},
	}
}

// createdDate drops the time of day from the "2006-01-02 15:04:05" stamp the EKS client has already formatted.
// A side panel is 30-60 cells wide, so the full stamp is a third of the row and RenderTableFit cuts it to "2026-01-02 1…", which reads as a broken value rather than a time. The whole stamp stays on the cluster's Config tab.
func createdDate(created string) string {
	date, _, _ := strings.Cut(created, " ")

	return date
}
