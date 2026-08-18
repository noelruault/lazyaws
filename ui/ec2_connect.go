package ui

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
)

func (gui *Gui) handleEC2Connect(g *gocui.Gui, v *gocui.View) error {
	if gui.readOnly() {
		return gui.refuseReadOnly("A shell on the instance")
	}

	inst, err := gui.Panels.EC2.GetSelectedItem()
	if err != nil {
		return nil
	}

	return gui.WithWaitingStatus("checking SSM connectivity", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		status, err := gui.Client.CheckSSMConnectivity(ctx, inst.ID)
		if err != nil {
			return err
		}
		if err := ssmConnectivityError(inst.ID, status); err != nil {
			return err
		}

		return gui.runSubprocess(buildSSMSessionCommand(inst.ID, gui.Client.GetRegion()))
	})
}

func ssmConnectivityError(instanceID string, status *aws.SSMConnectionStatus) error {
	if status != nil && status.Connected {
		return nil
	}
	return fmt.Errorf("instance %s is not reachable via SSM (agent not connected)", instanceID)
}

func buildSSMSessionCommand(instanceID, region string) *exec.Cmd {
	return exec.Command("aws", "ssm", "start-session", "--target", instanceID, "--region", region)
}
