package presentation

import (
	"github.com/noelruault/lazyaws/apps/aws"
)

func GetInstanceDisplayStrings(inst *aws.Instance) []string {
	name := inst.Name
	if name == "" {
		name = "(no name)"
	}
	return []string{
		StatusCell(inst.State, StatusStyleIcon),
		name,
		inst.ID,
		inst.InstanceType,
		inst.PrivateIP,
	}
}
