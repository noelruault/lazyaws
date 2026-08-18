package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/resources"
)

func (gui *Gui) ECSActions() []resources.Action {
	row, err := gui.Panels.ECS.GetSelectedItem()
	if err != nil {
		return nil
	}

	switch gui.ecsDrill.level {
	case ecsLevelClusters:
		return gui.ecsClusterActions(row)
	case ecsLevelServices:
		return gui.ecsServiceActions(row)
	default:
		return gui.ecsTaskActions(row)
	}
}

func (gui *Gui) ecsClusterActions(row *ecsRow) []resources.Action {
	if row.Kind != ecsRowKindCluster {
		return nil
	}
	cluster := row.Cluster

	return []resources.Action{{
		// AWS supplies the authoritative dependency error, avoiding a stale local pre-check.
		Name:    "Delete cluster",
		Mutates: true,
		Confirm: resources.ConfirmDangerous,
		Token:   cluster.Name,
		Run:     func(ctx context.Context, _ string) error { return gui.Client.DeleteECSCluster(ctx, cluster.Name) },
	}}
}

func (gui *Gui) ecsServiceActions(row *ecsRow) []resources.Action {
	if row.Kind != ecsRowKindService {
		return nil
	}

	service := row.Service
	cluster := gui.ecsDrill.cluster

	return []resources.Action{
		{
			Name:    "Scale to N tasks",
			Mutates: true,
			Prompt:  fmt.Sprintf("Desired count for %s (current: %d)", service.Name, service.DesiredCount),
			Run: func(ctx context.Context, input string) error {
				desired, err := parseDesiredCount(input)
				if err != nil {
					return err
				}
				return gui.Client.UpdateECSServiceDesiredCount(ctx, cluster, service.Name, desired)
			},
		},
		{
			Name:         "Force new deployment",
			Mutates:      true,
			Confirm:      resources.ConfirmSimple,
			Confirmation: fmt.Sprintf("Restart every task in %s on the task definition it already has?", service.Name),
			Run: func(ctx context.Context, _ string) error {
				return gui.Client.ForceNewECSDeployment(ctx, cluster, service.Name)
			},
		},
		{
			Name:    "Enable execute-command",
			Mutates: true,
			Confirm: resources.ConfirmSimple,
			// Existing tasks retain the Exec setting they started with.
			Confirmation: fmt.Sprintf("Turn ECS Exec on for %s? Only tasks started after this can be exec'd into.", service.Name),
			Run: func(ctx context.Context, _ string) error {
				return gui.Client.SetECSServiceExecuteCommand(ctx, cluster, service.Name, true)
			},
		},
		{
			Name:    "Delete service and its tasks",
			Mutates: true,
			Confirm: resources.ConfirmDangerous,
			Token:   service.Name,
			Run: func(ctx context.Context, _ string) error {
				return gui.Client.DeleteECSService(ctx, cluster, service.Name)
			},
		},
	}
}

func (gui *Gui) ecsTaskActions(row *ecsRow) []resources.Action {
	if row.Kind != ecsRowKindTask {
		return nil
	}

	task := row.Task
	cluster := gui.ecsDrill.cluster

	actions := make([]resources.Action, 0, len(task.Containers)+1)
	for _, container := range task.Containers {
		name := "Exec into " + container.Name
		if len(task.Containers) == 1 {
			name = "Exec into container"
		}

		actions = append(actions, resources.Action{
			Name: name,
			// A live shell is classified as mutating because it grants write-capable access.
			Mutates:      true,
			Confirm:      resources.ConfirmSimple,
			Confirmation: ecsExecPrompt(container.Name, task.ID),
			Run: func(_ context.Context, _ string) error {
				// The session owns the terminal until exit, so it must use suspend/resume instead of the action timeout.
				return gui.runSubprocess(gui.Client.ExecECSTask(cluster, task.Arn, container.Name))
			},
		})
	}

	return append(actions, resources.Action{
		Name:         "Stop task",
		Mutates:      true,
		Confirm:      resources.ConfirmSimple,
		Confirmation: ecsStopTaskQuestion(task, gui.ecsDrill.service),
		Run: func(ctx context.Context, _ string) error {
			return gui.Client.StopECSTask(ctx, cluster, task.Arn, "stopped from lazyaws")
		},
	})
}

// ecsStopTaskQuestion warns that service-managed tasks are replaced after stopping.
func ecsStopTaskQuestion(task *aws.ECSTask, service string) string {
	return fmt.Sprintf("Stop task %s? %s will start a replacement.", task.ID, service)
}

// parseDesiredCount rejects invalid input instead of guessing scale intent.
func parseDesiredCount(input string) (int32, error) {
	desired, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || desired < 0 {
		return 0, fmt.Errorf("desired count must be a whole number of tasks, zero or more, got %q", input)
	}
	if desired > ecsMaxDesiredCount {
		return 0, fmt.Errorf("desired count of %d looks like a typo; ECS caps a service at %d tasks", desired, ecsMaxDesiredCount)
	}

	return int32(desired), nil
}

// ecsMaxDesiredCount is ECS's own per-service task limit, used here only to catch a fat-fingered "1000" before it becomes a bill.
const ecsMaxDesiredCount = 5000
