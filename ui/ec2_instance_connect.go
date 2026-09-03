// Package ui uses ssh-keygen for EC2 Instance Connect to avoid hand-rolling OpenSSH key encoding.
package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
)

func (gui *Gui) handleEC2InstanceConnect(inst *aws.Instance) error {
	// Refused at the front door rather than three steps in: the flow prompts for a user, generates a key and pushes it with SendSSHPublicKey, and asking all that before refusing wastes the answer.
	if gui.readOnly() {
		return gui.refuseReadOnly("A shell on the instance")
	}

	host := ec2InstanceConnectHost(inst)
	if host == "" {
		return gui.createErrorPanel(fmt.Sprintf("instance %s has no reachable IP", inst.ID))
	}

	return gui.createPromptPanel(fmt.Sprintf("SSH user for %s (default: ec2-user)", inst.ID), func(g *gocui.Gui, v *gocui.View) error {
		user := ec2InstanceConnectUser(gui.trimmedContent(v))

		return gui.WithWaitingStatus("connecting via EC2 Instance Connect", func() error {
			keyDir, err := os.MkdirTemp("", "lazyaws-eic-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(keyDir)

			keyPath := keyDir + "/id_ed25519"
			if err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", keyPath).Run(); err != nil {
				return fmt.Errorf("failed to generate ephemeral SSH key: %w", err)
			}
			pubKey, err := os.ReadFile(keyPath + ".pub")
			if err != nil {
				return fmt.Errorf("failed to read ephemeral SSH public key: %w", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := gui.Client.SendSSHPublicKey(ctx, inst.ID, inst.AZ, user, strings.TrimSpace(string(pubKey))); err != nil {
				return err
			}

			return gui.runSubprocess(buildEC2InstanceConnectSSHCommand(keyPath, user, host))
		})
	})
}

// ec2InstanceConnectHost prefers public access and falls back to same-VPC private access.
func ec2InstanceConnectHost(inst *aws.Instance) string {
	if inst.PublicIP != "" {
		return inst.PublicIP
	}
	return inst.PrivateIP
}

func ec2InstanceConnectUser(input string) string {
	user := strings.TrimSpace(input)
	if user == "" {
		return "ec2-user"
	}
	return user
}

func buildEC2InstanceConnectSSHCommand(keyPath, user, host string) *exec.Cmd {
	return exec.Command("ssh",
		"-i", keyPath,
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "IdentitiesOnly=yes",
		fmt.Sprintf("%s@%s", user, host),
	)
}
