package presentation

import (
	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// InstanceWeights shares the slack between the name and the instance id, twice as much to the name.
// The id has to flex rather than size to its content: it is 19 cells, and with the icon, type and private IP that is the whole of a 40-cell side panel, which leaves a name column of zero width.
// A row that drops the name entirely to keep four narrower columns is the wrong trade — squeezing both text columns and cutting each with an ellipsis keeps every field on screen.
func InstanceWeights() []int {
	return []int{0, 2, 1, 0, 0}
}

func GetInstanceDisplayCells(inst *aws.Instance) []utils.Cell {
	name := inst.Name
	if name == "" {
		name = "(no name)"
	}

	return []utils.Cell{
		StatusCellFit(inst.State, StatusStyleIcon),
		{Text: name, Color: color.Bold},
		// The instance id is muted because it is the fallback identifier: you read it when the name is missing or ambiguous, not on every glance down the list.
		{Text: inst.ID, Color: color.Faint},
		{Text: inst.InstanceType},
		{Text: inst.PrivateIP},
	}
}
