package ec2

import (
	"context"

	"github.com/fuziontech/lazyaws/internal/aws"
)

// State contains EC2-specific UI and data state.
type State struct {
	Instances           []aws.Instance
	FilteredInstances   []aws.Instance
	SelectedIndex       int
	SelectedInstances   map[string]bool // for multi-select
	InstanceDetails     *aws.InstanceDetails
	InstanceStatus      *aws.InstanceStatus
	InstanceMetrics     *aws.InstanceMetrics
	SSMStatus           *aws.SSMConnectionStatus
	NeedRestore         bool
	ConfirmAction       string
	ConfirmInstanceID   string
	ShowingConfirmation bool
}

// LoadInstances fetches EC2 instances.
func LoadInstances(ctx context.Context, client *aws.Client) ([]aws.Instance, error) {
	if client == nil {
		return nil, nil
	}
	return client.ListInstances(ctx)
}

// LoadInstanceDetails fetches details for a single EC2 instance.
func LoadInstanceDetails(ctx context.Context, client *aws.Client, id string) (*aws.InstanceDetails, error) {
	if client == nil || id == "" {
		return nil, nil
	}
	return client.GetInstanceDetails(ctx, id)
}
