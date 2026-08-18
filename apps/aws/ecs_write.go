// Mutating ECS calls stay separate from readers so the write surface remains easy to audit.
// AWS converges asynchronously on all of these: the call returns once accepted, and the new state shows up on the next refresh rather than on return.
package aws

import (
	"context"
	"fmt"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

func updateDesiredCountInput(cluster, service string, desired int32) *ecs.UpdateServiceInput {
	return &ecs.UpdateServiceInput{
		Cluster:      sdkaws.String(cluster),
		Service:      sdkaws.String(service),
		DesiredCount: sdkaws.Int32(desired),
	}
}

func (c *Client) UpdateECSServiceDesiredCount(ctx context.Context, cluster, service string, desired int32) error {
	if _, err := c.ECS.UpdateService(ctx, updateDesiredCountInput(cluster, service, desired)); err != nil {
		return fmt.Errorf("failed to scale service %s to %d: %w", service, desired, err)
	}
	return nil
}

// forceNewDeploymentInput preserves the SDK's unusual non-pointer ForceNewDeployment field.
func forceNewDeploymentInput(cluster, service string) *ecs.UpdateServiceInput {
	return &ecs.UpdateServiceInput{
		Cluster:            sdkaws.String(cluster),
		Service:            sdkaws.String(service),
		ForceNewDeployment: true,
	}
}

func (c *Client) ForceNewECSDeployment(ctx context.Context, cluster, service string) error {
	if _, err := c.ECS.UpdateService(ctx, forceNewDeploymentInput(cluster, service)); err != nil {
		return fmt.Errorf("failed to force a new deployment of %s: %w", service, err)
	}
	return nil
}

func setExecuteCommandInput(cluster, service string, enabled bool) *ecs.UpdateServiceInput {
	return &ecs.UpdateServiceInput{
		Cluster:              sdkaws.String(cluster),
		Service:              sdkaws.String(service),
		EnableExecuteCommand: sdkaws.Bool(enabled),
	}
}

// SetECSServiceExecuteCommand affects only future tasks because running tasks retain their launch setting.
func (c *Client) SetECSServiceExecuteCommand(ctx context.Context, cluster, service string, enabled bool) error {
	if _, err := c.ECS.UpdateService(ctx, setExecuteCommandInput(cluster, service, enabled)); err != nil {
		return fmt.Errorf("failed to set execute-command on %s: %w", service, err)
	}
	return nil
}

// deleteServiceInput forces deletion so the gated action remains atomic for services with running tasks.
func deleteServiceInput(cluster, service string) *ecs.DeleteServiceInput {
	return &ecs.DeleteServiceInput{
		Cluster: sdkaws.String(cluster),
		Service: sdkaws.String(service),
		Force:   sdkaws.Bool(true),
	}
}

func (c *Client) DeleteECSService(ctx context.Context, cluster, service string) error {
	if _, err := c.ECS.DeleteService(ctx, deleteServiceInput(cluster, service)); err != nil {
		return fmt.Errorf("failed to delete service %s: %w", service, err)
	}
	return nil
}

// stopTaskInput carries the reason so ECS events distinguish operator action from a crash.
func stopTaskInput(cluster, taskArn, reason string) *ecs.StopTaskInput {
	return &ecs.StopTaskInput{
		Cluster: sdkaws.String(cluster),
		Task:    sdkaws.String(taskArn),
		Reason:  sdkaws.String(reason),
	}
}

func (c *Client) StopECSTask(ctx context.Context, cluster, taskArn, reason string) error {
	if _, err := c.ECS.StopTask(ctx, stopTaskInput(cluster, taskArn, reason)); err != nil {
		return fmt.Errorf("failed to stop task: %w", err)
	}
	return nil
}

// DeleteECSCluster trusts AWS's authoritative dependency refusal instead of a stale local pre-check.
func (c *Client) DeleteECSCluster(ctx context.Context, cluster string) error {
	input := &ecs.DeleteClusterInput{Cluster: sdkaws.String(cluster)}

	if _, err := c.ECS.DeleteCluster(ctx, input); err != nil {
		return fmt.Errorf("failed to delete cluster %s: %w", cluster, err)
	}
	return nil
}
