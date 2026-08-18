package presentation

import (
	"fmt"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

func GetECSClusterDisplayStrings(c *aws.ECSCluster) []string {
	return []string{
		StatusCell(c.Status, StatusStyleIcon),
		c.Name,
		utils.ColoredString(fmt.Sprintf("%d services", c.ActiveServicesCount), color.FgYellow),
		fmt.Sprintf("%d running / %d pending", c.RunningTasksCount, c.PendingTasksCount),
	}
}

func GetECSServiceDisplayStrings(s *aws.ECSService) []string {
	return []string{
		StatusCell(s.Status, StatusStyleIcon),
		s.Name,
		utils.ColoredString(s.LaunchType, color.FgYellow),
		fmt.Sprintf("%d/%d", s.RunningCount, s.DesiredCount),
	}
}

func GetECSTaskDisplayStrings(t *aws.ECSTask) []string {
	return []string{
		StatusCell(t.Status, StatusStyleIcon),
		utils.ColoredString(t.ID, color.FgMagenta),
		t.LaunchType,
	}
}
