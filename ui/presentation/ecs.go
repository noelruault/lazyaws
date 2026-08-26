package presentation

import (
	"fmt"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// ecsClusterActive is the one cluster status AWS treats as usable; everything else is a cluster being created, drained or deleted.
const ecsClusterActive = "ACTIVE"

// ECSClusterWeights sizes the cluster row so the name absorbs the slack and the counts and badge keep their full text.
func ECSClusterWeights() []int {
	return []int{0, 1, 0, 0, 0}
}

// ECSServiceWeights and ECSTaskWeights mirror ECSClusterWeights for the drilled-in levels, whose rows are a different shape.
func ECSServiceWeights() []int {
	return []int{0, 1, 0, 0}
}

func ECSTaskWeights() []int {
	return []int{0, 1, 0}
}

func GetECSClusterDisplayCells(c *aws.ECSCluster) []utils.Cell {
	return []utils.Cell{
		StatusCellFit(c.Status, StatusStyleIcon),
		{Text: c.Name},
		{Text: fmt.Sprintf("%d services", c.ActiveServicesCount), Color: color.FgYellow},
		{Text: fmt.Sprintf("%d running / %d pending", c.RunningTasksCount, c.PendingTasksCount)},
		ecsClusterBadge(c),
	}
}

// ecsClusterBadge answers "is this cluster fine right now" in one glance, which the raw status word cannot: a cluster is ACTIVE while its tasks are still coming up.
// Only an ACTIVE cluster can be healthy or deploying — pending tasks on a cluster that is being deleted are draining, not rolling out, and calling that "deploying" would read as progress.
func ecsClusterBadge(c *aws.ECSCluster) utils.Cell {
	switch {
	case c.Status == ecsClusterActive && c.PendingTasksCount == 0:
		return utils.Cell{Text: "● healthy", Color: color.FgGreen}
	case c.Status == ecsClusterActive:
		return utils.Cell{Text: "● deploying", Color: color.FgYellow}
	case c.Status == "":
		// DescribeClusters omits the status of a cluster it could not read; "● " alone would look like a rendering bug.
		return utils.Cell{Text: "● unknown", Color: color.FgRed}
	default:
		return utils.Cell{Text: "● " + c.Status, Color: color.FgRed}
	}
}

func GetECSServiceDisplayCells(s *aws.ECSService) []utils.Cell {
	return []utils.Cell{
		StatusCellFit(s.Status, StatusStyleIcon),
		{Text: s.Name},
		{Text: s.LaunchType, Color: color.FgYellow},
		{Text: fmt.Sprintf("%d/%d", s.RunningCount, s.DesiredCount)},
	}
}

// GetECSServiceDisplayStrings is the service row already coloured, for the cluster inspector's services table, which still lays out with RenderTable.
func GetECSServiceDisplayStrings(s *aws.ECSService) []string {
	cells := GetECSServiceDisplayCells(s)
	texts := make([]string, len(cells))
	for i, cell := range cells {
		texts[i] = cell.Rendered()
	}

	return texts
}

func GetECSTaskDisplayCells(t *aws.ECSTask) []utils.Cell {
	return []utils.Cell{
		StatusCellFit(t.Status, StatusStyleIcon),
		{Text: t.ID, Color: color.FgMagenta},
		{Text: t.LaunchType},
	}
}
