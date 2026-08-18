package ui

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/types"
)

func (gui *Gui) handleECSExec(g *gocui.Gui, v *gocui.View) error {
	if gui.readOnly() {
		return gui.refuseReadOnly("A shell inside a container")
	}

	if gui.ecsDrill.level != ecsLevelTasks {
		return nil
	}
	row, err := gui.Panels.ECS.GetSelectedItem()
	if err != nil || row.Kind != ecsRowKindTask {
		return nil
	}
	task := row.Task
	if len(task.Containers) == 0 {
		return nil
	}
	if len(task.Containers) == 1 {
		return gui.confirmECSExec(task, task.Containers[0].Name)
	}

	items := make([]*types.MenuItem, len(task.Containers))
	for i, c := range task.Containers {
		items[i] = &types.MenuItem{Label: c.Name, OnPress: func() error {
			return gui.confirmECSExec(task, c.Name)
		}}
	}
	return gui.Menu(CreateMenuOptions{Title: "Exec into container", Items: items})
}

// confirmECSExec suspends the TUI because the interactive session owns the terminal.
func (gui *Gui) confirmECSExec(task *aws.ECSTask, containerName string) error {
	prompt := ecsExecPrompt(containerName, task.ID)
	cluster := gui.ecsDrill.cluster
	return gui.createConfirmationPanel("ECS Exec", prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus("starting ECS exec session", func() error {
			return gui.runSubprocess(gui.Client.ExecECSTask(cluster, task.Arn, containerName))
		})
	}, nil)
}

// ecsExecPrompt names both container and task so shell access is deliberate.
func ecsExecPrompt(containerName, taskID string) string {
	return color.New(color.FgRed).SprintFunc()(fmt.Sprintf("Exec into container %s on task %s? Opens an interactive shell.", containerName, taskID))
}
