package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	logTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// ECSCluster represents a simplified ECS cluster summary.
type ECSCluster struct {
	Name                     string
	Arn                      string
	Status                   string
	RunningTasksCount        int32
	PendingTasksCount        int32
	ActiveServicesCount      int32
	RegisteredContainerCount int32
}

// ECSDeployment captures deployment summary for a service.
type ECSDeployment struct {
	Status  string
	Desired int32
	Running int32
	Pending int32
	Created *time.Time
}

// ECSEvent is a recent service event.
type ECSEvent struct {
	Message string
	When    *time.Time
}

// ECSService represents a service with health and deployment data.
type ECSService struct {
	Name                       string
	Arn                        string
	Status                     string
	TaskDefinition             string
	LaunchType                 string
	DesiredCount               int32
	RunningCount               int32
	PendingCount               int32
	HealthCheckGracePeriodSecs int32
	Deployments                []ECSDeployment
	Events                     []ECSEvent
}

// ECSPortMapping captures container port mappings.
type ECSPortMapping struct {
	ContainerPort int32
	HostPort      int32
	Protocol      string
}

// ECSContainer captures container-level status.
type ECSContainer struct {
	Name         string
	LastStatus   string
	HealthStatus string
	PrivateIPs   []string
	Ports        []ECSPortMapping
}

// ECSAttachment captures network attachment info.
type ECSAttachment struct {
	Type    string
	Details map[string]string
}

// ECSTask represents a task with configuration/health.
type ECSTask struct {
	Arn              string
	ID               string
	Status           string
	Health           string
	CPU              string
	Memory           string
	TaskDefinition   string
	LaunchType       string
	AvailabilityZone string
	CreatedAt        *time.Time
	StartedAt        *time.Time
	Containers       []ECSContainer
	Attachments      []ECSAttachment
}

// ECSLogEvent captures a single log line.
type ECSLogEvent struct {
	Timestamp time.Time
	Message   string
}

// ECSLogStream groups logs for one container.
type ECSLogStream struct {
	Container string
	LogGroup  string
	LogStream string
	Events    []ECSLogEvent
}

// ListECSClusters returns ECS cluster summaries (name, status, counts).
func (c *Client) ListECSClusters(ctx context.Context) ([]ECSCluster, error) {
	if c.ECS == nil {
		return nil, fmt.Errorf("ECS client not initialized")
	}

	// Ensure we have a reasonable timeout
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}

	var clusters []ECSCluster
	var nextToken *string
	for {
		out, err := c.ECS.ListClusters(ctx, &ecs.ListClustersInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list ECS clusters: %w", err)
		}
		if len(out.ClusterArns) == 0 {
			break
		}

		// Describe clusters to get names/status/counters
		descOut, err := c.ECS.DescribeClusters(ctx, &ecs.DescribeClustersInput{
			Clusters: out.ClusterArns,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to describe ECS clusters: %w", err)
		}

		for _, cl := range descOut.Clusters {
			cluster := ECSCluster{
				Name:   getString(cl.ClusterName),
				Arn:    getString(cl.ClusterArn),
				Status: getString(cl.Status),
			}
			cluster.RunningTasksCount = cl.RunningTasksCount
			cluster.PendingTasksCount = cl.PendingTasksCount
			cluster.ActiveServicesCount = cl.ActiveServicesCount
			cluster.RegisteredContainerCount = cl.RegisteredContainerInstancesCount
			clusters = append(clusters, cluster)
		}

		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	return clusters, nil
}

// ListECSServices returns ECS services for a cluster with health metadata.
func (c *Client) ListECSServices(ctx context.Context, clusterName string) ([]ECSService, error) {
	if c.ECS == nil {
		return nil, fmt.Errorf("ECS client not initialized")
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
	}

	var services []ECSService
	var nextToken *string
	for {
		listOut, err := c.ECS.ListServices(ctx, &ecs.ListServicesInput{
			Cluster:   &clusterName,
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list ECS services: %w", err)
		}
		if len(listOut.ServiceArns) == 0 {
			break
		}

		descOut, err := c.ECS.DescribeServices(ctx, &ecs.DescribeServicesInput{
			Cluster:  &clusterName,
			Services: listOut.ServiceArns,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to describe ECS services: %w", err)
		}

		for _, svc := range descOut.Services {
			service := ECSService{
				Name:                       getString(svc.ServiceName),
				Arn:                        getString(svc.ServiceArn),
				Status:                     getString(svc.Status),
				TaskDefinition:             getString(svc.TaskDefinition),
				DesiredCount:               svc.DesiredCount,
				RunningCount:               svc.RunningCount,
				PendingCount:               svc.PendingCount,
				LaunchType:                 string(svc.LaunchType),
				HealthCheckGracePeriodSecs: getInt32Value(svc.HealthCheckGracePeriodSeconds),
			}

			// Deployments
			for _, dep := range svc.Deployments {
				service.Deployments = append(service.Deployments, ECSDeployment{
					Status:  getString(dep.Status),
					Desired: dep.DesiredCount,
					Running: dep.RunningCount,
					Pending: dep.PendingCount,
					Created: dep.CreatedAt,
				})
			}

			// Events (cap to most recent 5)
			events := svc.Events
			if len(events) > 5 {
				events = events[:5]
			}
			for _, ev := range events {
				service.Events = append(service.Events, ECSEvent{
					Message: getString(ev.Message),
					When:    ev.CreatedAt,
				})
			}

			services = append(services, service)
		}

		if listOut.NextToken == nil {
			break
		}
		nextToken = listOut.NextToken
	}

	return services, nil
}

// ListECSTasks returns tasks for a service with useful configuration details.
func (c *Client) ListECSTasks(ctx context.Context, clusterName, serviceName string) ([]ECSTask, error) {
	if c.ECS == nil {
		return nil, fmt.Errorf("ECS client not initialized")
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
	}

	listOut, err := c.ECS.ListTasks(ctx, &ecs.ListTasksInput{
		Cluster:     &clusterName,
		ServiceName: &serviceName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list ECS tasks: %w", err)
	}
	if len(listOut.TaskArns) == 0 {
		return nil, nil
	}

	descOut, err := c.ECS.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: &clusterName,
		Tasks:   listOut.TaskArns,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe ECS tasks: %w", err)
	}

	var tasks []ECSTask
	for _, t := range descOut.Tasks {
		task := ECSTask{
			Arn:              getString(t.TaskArn),
			ID:               extractTaskID(getString(t.TaskArn)),
			Status:           getString(t.LastStatus),
			Health:           string(t.HealthStatus),
			CPU:              getString(t.Cpu),
			Memory:           getString(t.Memory),
			TaskDefinition:   getString(t.TaskDefinitionArn),
			LaunchType:       string(t.LaunchType),
			AvailabilityZone: getString(t.AvailabilityZone),
			CreatedAt:        t.CreatedAt,
			StartedAt:        t.StartedAt,
		}

		// Containers
		for _, ctn := range t.Containers {
			container := ECSContainer{
				Name:         getString(ctn.Name),
				LastStatus:   getString(ctn.LastStatus),
				HealthStatus: string(ctn.HealthStatus),
			}
			for _, binding := range ctn.NetworkBindings {
				container.Ports = append(container.Ports, ECSPortMapping{
					ContainerPort: getInt32Value(binding.ContainerPort),
					HostPort:      getInt32Value(binding.HostPort),
					Protocol:      string(binding.Protocol),
				})
			}
			for _, iface := range ctn.NetworkInterfaces {
				if iface.PrivateIpv4Address != nil {
					container.PrivateIPs = append(container.PrivateIPs, *iface.PrivateIpv4Address)
				}
			}
			task.Containers = append(task.Containers, container)
		}

		// Attachments for network details
		for _, att := range t.Attachments {
			details := make(map[string]string)
			for _, d := range att.Details {
				if d.Name != nil && d.Value != nil {
					details[*d.Name] = *d.Value
				}
			}
			task.Attachments = append(task.Attachments, ECSAttachment{
				Type:    getString(att.Type),
				Details: details,
			})
		}

		tasks = append(tasks, task)
	}

	return tasks, nil
}

// GetECSTaskLogs fetches CloudWatch logs for a task (per container) if configured.
func (c *Client) GetECSTaskLogs(ctx context.Context, clusterName, taskArn string, limit int32) ([]ECSLogStream, error) {
	if c.ECS == nil || c.CloudWatch == nil {
		return nil, fmt.Errorf("ECS or CloudWatchLogs client not initialized")
	}

	if limit <= 0 {
		limit = 50
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
	}

	taskID := extractTaskID(taskArn)

	// Describe the task to get the task definition
	taskDesc, err := c.ECS.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: &clusterName,
		Tasks:   []string{taskArn},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe ECS task for logs: %w", err)
	}
	if len(taskDesc.Tasks) == 0 {
		return nil, fmt.Errorf("task not found")
	}
	task := taskDesc.Tasks[0]

	// Describe task definition to find log configuration
	tdOut, err := c.ECS.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: task.TaskDefinitionArn,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe task definition for logs: %w", err)
	}

	var streams []ECSLogStream
	for _, cd := range tdOut.TaskDefinition.ContainerDefinitions {
		if cd.LogConfiguration == nil || cd.LogConfiguration.LogDriver != ecsTypes.LogDriverAwslogs {
			continue
		}
		opts := cd.LogConfiguration.Options
		logGroup := opts["awslogs-group"]
		streamPrefix := opts["awslogs-stream-prefix"]
		if logGroup == "" || streamPrefix == "" {
			continue
		}
		logStream := fmt.Sprintf("%s/%s/%s", streamPrefix, getString(cd.Name), taskID)

		events, err := c.fetchLogEvents(ctx, logGroup, logStream, limit)
		if err != nil {
			// If specific stream missing, try the most recent stream in group
			if streamsList, derr := c.findRecentLogStream(ctx, logGroup); derr == nil && streamsList != "" {
				if ev, ferr := c.fetchLogEvents(ctx, logGroup, streamsList, limit); ferr == nil {
					events = ev
					logStream = streamsList
				}
			}
		}

		streams = append(streams, ECSLogStream{
			Container: getString(cd.Name),
			LogGroup:  logGroup,
			LogStream: logStream,
			Events:    events,
		})
	}

	return streams, nil
}

func (c *Client) fetchLogEvents(ctx context.Context, group, stream string, limit int32) ([]ECSLogEvent, error) {
	out, err := c.CloudWatchLogs.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  &group,
		LogStreamName: &stream,
		Limit:         &limit,
		StartFromHead: sdkaws.Bool(false),
	})
	if err != nil {
		return nil, err
	}

	var events []ECSLogEvent
	for _, ev := range out.Events {
		if ev.Timestamp == nil || ev.Message == nil {
			continue
		}
		events = append(events, ECSLogEvent{
			Timestamp: time.UnixMilli(*ev.Timestamp),
			Message:   *ev.Message,
		})
	}
	return events, nil
}

func (c *Client) findRecentLogStream(ctx context.Context, group string) (string, error) {
	out, err := c.CloudWatchLogs.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName:        &group,
		OrderBy:             logTypes.OrderByLastEventTime,
		Descending:          sdkaws.Bool(true),
		Limit:               getInt32Ptr(5),
		LogStreamNamePrefix: nil,
	})
	if err != nil {
		return "", err
	}
	if len(out.LogStreams) == 0 || out.LogStreams[0].LogStreamName == nil {
		return "", fmt.Errorf("no log streams")
	}
	return *out.LogStreams[0].LogStreamName, nil
}

// extractTaskID returns the final segment of a task ARN.
func extractTaskID(taskArn string) string {
	parts := strings.Split(taskArn, "/")
	if len(parts) == 0 {
		return taskArn
	}
	return parts[len(parts)-1]
}

func getInt32Value(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
}
