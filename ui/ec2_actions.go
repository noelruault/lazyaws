package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/resources"
)

// EC2Actions defers state reload to refresh because AWS transitions are asynchronous.
func (gui *Gui) EC2Actions() []resources.Action {
	inst, err := gui.Panels.EC2.GetSelectedItem()
	if err != nil {
		return nil
	}

	return []resources.Action{
		{Name: "Start", Mutates: true, Run: ec2Call(gui.Client.StartInstance, inst.ID)},
		{Name: "Stop", Mutates: true, Run: ec2Call(gui.Client.StopInstance, inst.ID)},
		{Name: "Reboot", Mutates: true, Run: ec2Call(gui.Client.RebootInstance, inst.ID)},
		{
			Name:    "Terminate",
			Mutates: true,
			// Irreversible termination requires the instance's visible identity instead of a keystroke.
			Confirm: resources.ConfirmDangerous,
			Token:   ec2InstanceToken(inst),
			Run:     ec2Call(gui.Client.TerminateInstance, inst.ID),
		},
		{
			Name:    "Change instance type",
			Mutates: true,
			Prompt:  fmt.Sprintf("New instance type for %s (current: %s)", inst.ID, inst.InstanceType),
			// Stop, modify, and restart can take minutes.
			Timeout: 5 * time.Minute,
			Run: func(ctx context.Context, newType string) error {
				if newType == "" || newType == inst.InstanceType {
					return nil
				}
				return gui.Client.ChangeInstanceType(ctx, inst.ID, newType)
			},
		},
		{
			Name:    "Create image",
			Mutates: true,
			Prompt:  "Image name for " + inst.ID,
			Timeout: 5 * time.Minute,
			Run: func(ctx context.Context, imageName string) error {
				if imageName == "" {
					return nil
				}
				_, err := gui.Client.CreateImageFromInstance(ctx, inst.ID, imageName)
				return err
			},
		},
		{Name: "Create snapshot", Mutates: true, Run: gui.ec2CreateSnapshot(inst)},
		{Name: "Manage Elastic IPs", Mutates: true, Run: gui.ec2ManageEIPs(inst)},
		{Name: "View/edit user data", Mutates: true, Run: gui.ec2ViewEditUserData(inst)},
		{
			Name:    "Connect via EC2 Instance Connect",
			Mutates: true,
			// Run executes off the UI thread, so prompt creation must be queued.
			Run: func(context.Context, string) error {
				gui.g.Update(func(*gocui.Gui) error { return gui.handleEC2InstanceConnect(inst) })
				return nil
			},
		},
		{
			Name:    "Toggle termination protection",
			Mutates: true,
			// Reading protection while building the menu would block the UI key handler on AWS.
			Run: func(ctx context.Context, _ string) error {
				enabled, err := gui.Client.GetInstanceTerminationProtection(ctx, inst.ID)
				if err != nil {
					return err
				}
				return gui.Client.SetInstanceTerminationProtection(ctx, inst.ID, !enabled)
			},
		},
	}
}

// ec2InstanceToken combines visible identity without encouraging pasted ARNs.
func ec2InstanceToken(inst *aws.Instance) string {
	if inst.Name == "" {
		return inst.ID
	}
	return inst.Name + " " + inst.ID
}

func ec2Call(call func(context.Context, string) error, instanceID string) func(context.Context, string) error {
	return func(ctx context.Context, _ string) error {
		return call(ctx, instanceID)
	}
}

// ec2CreateSnapshot fetches details because flat panel rows omit volumes.
func (gui *Gui) ec2CreateSnapshot(inst *aws.Instance) func(context.Context, string) error {
	return func(ctx context.Context, _ string) error {
		details, err := gui.Client.GetInstanceDetails(ctx, inst.ID)
		if err != nil {
			return err
		}

		devices := ec2VolumeDevices(details.BlockDevices)
		switch len(devices) {
		case 0:
			return fmt.Errorf("instance %s has no EBS volumes", inst.ID)
		case 1:
			return gui.snapshotVolume(ctx, devices[0])
		}

		actions := make([]resources.Action, len(devices))
		for i, device := range devices {
			actions[i] = resources.Action{
				Name:    fmt.Sprintf("%s (%s)", device.DeviceName, device.VolumeID),
				Mutates: true,
				Timeout: 30 * time.Second,
				Run:     func(ctx context.Context, _ string) error { return gui.snapshotVolume(ctx, device) },
			}
		}

		gui.g.Update(func(g *gocui.Gui) error {
			return gui.Menu(CreateMenuOptions{Title: "Create snapshot — choose volume", Items: gui.actionMenuItems(actions)})
		})

		return nil
	}
}

func (gui *Gui) snapshotVolume(ctx context.Context, device aws.BlockDevice) error {
	_, err := gui.Client.CreateVolumeSnapshot(ctx, device.VolumeID, "lazyaws snapshot of "+device.DeviceName)
	return err
}

// ec2VolumeDevices excludes instance-store devices because they have no EBS volume.
func ec2VolumeDevices(devices []aws.BlockDevice) []aws.BlockDevice {
	out := make([]aws.BlockDevice, 0, len(devices))
	for _, d := range devices {
		if d.VolumeID != "" {
			out = append(out, d)
		}
	}
	return out
}

func (gui *Gui) ec2ManageEIPs(inst *aws.Instance) func(context.Context, string) error {
	return func(ctx context.Context, _ string) error {
		details, err := gui.Client.GetInstanceDetails(ctx, inst.ID)
		if err != nil {
			return err
		}

		actions := []resources.Action{{
			Name:    "Associate EIP",
			Mutates: true,
			Prompt:  "Allocation ID of EIP to associate",
			Run: func(ctx context.Context, allocationID string) error {
				if allocationID == "" {
					return nil
				}
				return gui.Client.AssociateElasticIP(ctx, inst.ID, allocationID)
			},
		}}

		for _, eip := range details.ElasticIPs {
			actions = append(actions, resources.Action{
				Name:         "Disassociate " + eip.PublicIP,
				Mutates:      true,
				Confirm:      resources.ConfirmSimple,
				Confirmation: fmt.Sprintf("Remove %s from this instance?", eip.PublicIP),
				Run: func(ctx context.Context, _ string) error {
					return gui.Client.DisassociateElasticIP(ctx, eip.AssociationID)
				},
			})
		}

		gui.g.Update(func(g *gocui.Gui) error {
			return gui.Menu(CreateMenuOptions{Title: "Manage Elastic IPs", Items: gui.actionMenuItems(actions)})
		})

		return nil
	}
}

// ec2ViewEditUserData offers edits only while AWS permits them on stopped instances.
func (gui *Gui) ec2ViewEditUserData(inst *aws.Instance) func(context.Context, string) error {
	return func(ctx context.Context, _ string) error {
		userData, err := gui.Client.GetInstanceUserData(ctx, inst.ID)
		if err != nil {
			return err
		}

		var edit func(*gocui.Gui, *gocui.View) error
		if inst.State == "stopped" {
			edit = func(g *gocui.Gui, v *gocui.View) error {
				return gui.runAction(resources.Action{
					Name:    "Set user data",
					Mutates: true,
					Prompt:  "Edit user data (base64 or plaintext)",
					Run: func(ctx context.Context, newUserData string) error {
						if newUserData == "" || newUserData == userData {
							return nil
						}
						return gui.Client.SetInstanceUserData(ctx, inst.ID, newUserData)
					},
				})
			}
		}

		// The secondary callback doubles as Edit because the primary callback dismisses the viewer.
		return gui.createConfirmationPanel("User data for "+inst.ID, userData, func(*gocui.Gui, *gocui.View) error { return nil }, edit)
	}
}
