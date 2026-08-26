package presentation

import (
	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// VPCWeights gives the label all the slack and sizes the vpc id to its content.
// A vpc id is a fixed 21 cells, so a proportional share of a wide panel under-pays it and cuts it while the label sits on idle padding; sizing it to content shows it whole wherever it fits, and the label's flexibleFloor is what keeps a row readable when it does not.
func VPCWeights() []int {
	return []int{0, 0, 1, 0}
}

// GetVPCDisplayCells leads with the CIDR because that is what a VPC gets recognised by when tracing whether two networks can reach each other; the name is a label on top of it, and often absent.
func GetVPCDisplayCells(v *aws.VPC) []utils.Cell {
	label := v.Name
	if label == "" {
		label = "(no name)"
	}
	if v.IsDefault {
		label += " (default)"
	}

	return []utils.Cell{
		StatusCellFit(v.State, StatusStyleIcon),
		{Text: v.CIDR, Color: color.Bold},
		{Text: label},
		// The vpc id is the fallback identifier: you read it when writing a rule or a query, not on every glance down the list.
		{Text: v.ID, Color: color.Faint},
	}
}

// GetVPCEndpointDisplayStrings labels the row by service rather than by id, because endpoints are usually untagged and one id looks like the next.
func GetVPCEndpointDisplayStrings(e *aws.VPCEndpoint) []string {
	label := e.Name
	if label == "" {
		label = e.ShortService()
	}

	return []string{
		StatusCell(e.State, StatusStyleIcon),
		label,
		e.ID,
		e.Type,
		e.VpcID,
	}
}
