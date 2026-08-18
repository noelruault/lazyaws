package aws

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type SSMConnectionStatus struct {
	InstanceID   string
	Connected    bool
	PingStatus   string
	AgentVersion string
	PlatformType string
	PlatformName string
	LastPingTime string
}

// CheckSSMConnectivity returns a disconnected status, not an error, when SSM has no record.
func (c *Client) CheckSSMConnectivity(ctx context.Context, instanceID string) (*SSMConnectionStatus, error) {
	filterKey := "InstanceIds"
	input := &ssm.DescribeInstanceInformationInput{
		Filters: []types.InstanceInformationStringFilter{
			{
				Key:    &filterKey,
				Values: []string{instanceID},
			},
		},
	}

	result, err := c.SSM.DescribeInstanceInformation(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe instance information: %w", err)
	}

	status := &SSMConnectionStatus{
		InstanceID: instanceID,
		Connected:  false,
	}

	if len(result.InstanceInformationList) == 0 {
		return status, nil
	}

	info := result.InstanceInformationList[0]
	status.Connected = true
	status.PingStatus = string(info.PingStatus)
	status.AgentVersion = getString(info.AgentVersion)
	status.PlatformType = string(info.PlatformType)
	status.PlatformName = getString(info.PlatformName)

	if info.LastPingDateTime != nil {
		status.LastPingTime = info.LastPingDateTime.Format("2006-01-02 15:04:05")
	}

	return status, nil
}

func (c *Client) LaunchSSMSession(instanceID string, region string) error {
	terminal := detectTerminal()
	if terminal == "" {
		return fmt.Errorf("could not detect terminal emulator")
	}

	ssmCommand := fmt.Sprintf("aws ssm start-session --target %s --region %s", instanceID, region)

	var cmd *exec.Cmd
	switch terminal {
	case "ghostty":
		cmd = exec.Command("ghostty", "-e", "bash", "-c", ssmCommand+"; exec bash")
	case "gnome-terminal":
		cmd = exec.Command("gnome-terminal", "--", "bash", "-c", ssmCommand+"; exec bash")
	case "xterm":
		cmd = exec.Command("xterm", "-e", "bash -c '"+ssmCommand+"; exec bash'")
	case "konsole":
		cmd = exec.Command("konsole", "-e", "bash", "-c", ssmCommand+"; exec bash")
	case "xfce4-terminal":
		cmd = exec.Command("xfce4-terminal", "-e", "bash -c '"+ssmCommand+"; exec bash'")
	case "alacritty":
		cmd = exec.Command("alacritty", "-e", "bash", "-c", ssmCommand+"; exec bash")
	case "kitty":
		cmd = exec.Command("kitty", "bash", "-c", ssmCommand+"; exec bash")
	case "terminator":
		cmd = exec.Command("terminator", "-e", "bash -c '"+ssmCommand+"; exec bash'")
	default:
		return fmt.Errorf("unsupported terminal: %s", terminal)
	}

	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to launch terminal: %w", err)
	}

	return nil
}

func detectTerminal() string {
	if os.Getenv("TERM_PROGRAM") == "ghostty" {
		return "ghostty"
	}

	terminals := []string{
		"ghostty",
		"gnome-terminal",
		"konsole",
		"xfce4-terminal",
		"xterm",
		"alacritty",
		"kitty",
		"terminator",
	}

	for _, term := range terminals {
		if _, err := exec.LookPath(term); err == nil {
			return term
		}
	}

	if term := os.Getenv("TERM"); term != "" {
		return term
	}

	return ""
}

func (c *Client) StartPortForward(instanceID string, region string, localPort int, remotePort int, remoteHost string) error {
	terminal := detectTerminal()
	if terminal == "" {
		return fmt.Errorf("could not detect terminal emulator")
	}

	var ssmCommand string
	if remoteHost == "" || remoteHost == "localhost" {
		ssmCommand = fmt.Sprintf("aws ssm start-session --target %s --region %s --document-name AWS-StartPortForwardingSession --parameters 'localPortNumber=%d,portNumber=%d'",
			instanceID, region, localPort, remotePort)
	} else {
		ssmCommand = fmt.Sprintf("aws ssm start-session --target %s --region %s --document-name AWS-StartPortForwardingSessionToRemoteHost --parameters 'host=%s,localPortNumber=%d,portNumber=%d'",
			instanceID, region, remoteHost, localPort, remotePort)
	}

	var cmd *exec.Cmd
	switch terminal {
	case "ghostty":
		cmd = exec.Command("ghostty", "-e", "bash", "-c", ssmCommand+"; echo 'Press Enter to close'; read")
	case "gnome-terminal":
		cmd = exec.Command("gnome-terminal", "--", "bash", "-c", ssmCommand+"; echo 'Press Enter to close'; read")
	case "xterm":
		cmd = exec.Command("xterm", "-e", "bash -c '"+ssmCommand+"; echo Press Enter to close; read'")
	case "konsole":
		cmd = exec.Command("konsole", "-e", "bash", "-c", ssmCommand+"; echo 'Press Enter to close'; read")
	case "xfce4-terminal":
		cmd = exec.Command("xfce4-terminal", "-e", "bash -c '"+ssmCommand+"; echo Press Enter to close; read'")
	case "alacritty":
		cmd = exec.Command("alacritty", "-e", "bash", "-c", ssmCommand+"; echo 'Press Enter to close'; read")
	case "kitty":
		cmd = exec.Command("kitty", "bash", "-c", ssmCommand+"; echo 'Press Enter to close'; read")
	case "terminator":
		cmd = exec.Command("terminator", "-e", "bash -c '"+ssmCommand+"; echo Press Enter to close; read'")
	default:
		return fmt.Errorf("unsupported terminal: %s", terminal)
	}

	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to launch terminal: %w", err)
	}

	return nil
}
