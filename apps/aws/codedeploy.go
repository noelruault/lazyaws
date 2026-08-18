package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/codedeploy"
)

type ECSCodeDeployStatus struct {
	ApplicationName      string
	DeploymentGroupName  string
	LastAttemptedStatus  string
	LastAttemptedAt      *time.Time
	LastSuccessfulStatus string
	LastSuccessfulAt     *time.Time
}

// GetECSCodeDeployStatus scans deployment groups because CodeDeploy has no ECS-service reverse lookup.
func (c *Client) GetECSCodeDeployStatus(ctx context.Context, clusterName, serviceName string) (*ECSCodeDeployStatus, error) {
	if c.CodeDeploy == nil {
		return nil, fmt.Errorf("CodeDeploy client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 20*time.Second)
	defer cancel()

	var appToken *string
	for {
		apps, err := c.CodeDeploy.ListApplications(timeoutCtx, &codedeploy.ListApplicationsInput{NextToken: appToken})
		if err != nil {
			return nil, fmt.Errorf("failed to list CodeDeploy applications: %w", err)
		}

		for _, appName := range apps.Applications {
			status, err := c.findECSDeploymentGroup(timeoutCtx, appName, clusterName, serviceName)
			if err != nil {
				return nil, err
			}
			if status != nil {
				return status, nil
			}
		}

		if apps.NextToken == nil {
			break
		}
		appToken = apps.NextToken
	}

	return nil, nil
}

func (c *Client) findECSDeploymentGroup(ctx context.Context, appName, clusterName, serviceName string) (*ECSCodeDeployStatus, error) {
	var dgToken *string
	for {
		dgs, err := c.CodeDeploy.ListDeploymentGroups(ctx, &codedeploy.ListDeploymentGroupsInput{
			ApplicationName: &appName,
			NextToken:       dgToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list deployment groups for %s: %w", appName, err)
		}

		for _, dgName := range dgs.DeploymentGroups {
			out, err := c.CodeDeploy.GetDeploymentGroup(ctx, &codedeploy.GetDeploymentGroupInput{
				ApplicationName:     &appName,
				DeploymentGroupName: &dgName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to describe deployment group %s/%s: %w", appName, dgName, err)
			}

			info := out.DeploymentGroupInfo
			if info == nil {
				continue
			}
			for _, svc := range info.EcsServices {
				if getString(svc.ClusterName) != clusterName || getString(svc.ServiceName) != serviceName {
					continue
				}

				status := &ECSCodeDeployStatus{
					ApplicationName:     appName,
					DeploymentGroupName: dgName,
				}
				if info.LastAttemptedDeployment != nil {
					status.LastAttemptedStatus = string(info.LastAttemptedDeployment.Status)
					status.LastAttemptedAt = info.LastAttemptedDeployment.CreateTime
				}
				if info.LastSuccessfulDeployment != nil {
					status.LastSuccessfulStatus = string(info.LastSuccessfulDeployment.Status)
					status.LastSuccessfulAt = info.LastSuccessfulDeployment.CreateTime
				}
				return status, nil
			}
		}

		if dgs.NextToken == nil {
			break
		}
		dgToken = dgs.NextToken
	}

	return nil, nil
}
