package ecs

import "github.com/noelruault/lazyaws/internal/aws"

// State holds ECS-specific UI state.
type State struct {
	Clusters      []aws.ECSCluster
	Filtered      []aws.ECSCluster
	SelectedIndex int

	CurrentCluster string

	Services       []aws.ECSService
	FilteredSvcs   []aws.ECSService
	SelectedSvc    int
	CurrentService string

	Tasks         []aws.ECSTask
	FilteredTasks []aws.ECSTask
	SelectedTask  int

	TaskLogs []aws.ECSLogStream
}
