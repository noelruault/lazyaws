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

	client := gui.awsClient()

	// Spawned because this is a key handler on the UI loop and the session it opens lasts as long as the user keeps it.
	go func() {
		_ = gui.WithWaitingStatus("checking SSM connectivity", func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			status, err := client.CheckSSMConnectivity(ctx, inst.ID)
			if err != nil {
				return err
			}
			if err := ssmConnectivityError(inst.ID, status); err != nil {
				return err
			}

			return gui.runSubprocess(buildSSMSessionCommand(inst.ID, client.GetRegion()))
		})
	}()

	return nil
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
