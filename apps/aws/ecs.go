package aws

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

type ECSCapacityProviderStrategy struct {
	CapacityProvider string
	Weight           int32
	Base             int32
}

// ECSClusterStatistics splits a cluster's task and service counts by launch type.
// DescribeClusters only fills these when asked for the STATISTICS field; without it every count is zero, which is indistinguishable from an idle cluster.
type ECSClusterStatistics struct {
	RunningEC2Tasks         int32
	RunningFargateTasks     int32
	PendingEC2Tasks         int32
	PendingFargateTasks     int32
	ActiveEC2Services       int32
	ActiveFargateServices   int32
	DrainingEC2Services     int32
	DrainingFargateServices int32
}

type ECSCluster struct {
	Name                         string
	Arn                          string
	Status                       string
	RunningTasksCount            int32
	PendingTasksCount            int32
	ActiveServicesCount          int32
	RegisteredContainerCount     int32
	ConsoleURL                   string
	CapacityProviders            []string
	DefaultCapacityProviderStrat []ECSCapacityProviderStrategy
	Statistics                   ECSClusterStatistics
	// ContainerInsights is the cluster's containerInsights setting verbatim (enabled, enhanced or disabled), empty when the cluster was described without the SETTINGS field.
	// Empty is not disabled: it means unknown, and the difference decides whether the Insights metric namespace is worth querying at all.
	ContainerInsights string
}

type ECSDeployment struct {
	Status  string
	Desired int32
	Running int32
	Pending int32
	Created *time.Time
}

type ECSEvent struct {
	Message string
	When    *time.Time
}

type ECSService struct {
	Name                       string
	Arn                        string
	Status                     string
	TaskDefinition             string
	Cluster                    string
	Region                     string
	CreatedAt                  *time.Time
	LaunchType                 string
	DesiredCount               int32
	RunningCount               int32
	PendingCount               int32
	HealthCheckGracePeriodSecs int32
	Deployments                []ECSDeployment
	Events                     []ECSEvent
	LoadBalancers              []ECSLoadBalancer
	ConsoleURL                 string
	DeploymentController       string // ECS, CODE_DEPLOY, or EXTERNAL
	CircuitBreakerEnabled      bool
	CircuitBreakerRollback     bool
}

type ECSPortMapping struct {
	ContainerPort int32
	HostPort      int32
	Protocol      string
}

type ECSContainer struct {
	Name         string
	LastStatus   string
	HealthStatus string
	ImageURI     string
	ImageDigest  string
	RuntimeID    string
	CPU          float64
	MemoryHardMB int32
	MemorySoftMB int32
	PrivateIPs   []string
	Ports        []ECSPortMapping
}

type ECSAttachment struct {
	Type    string
	Details map[string]string
}

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
	ConsoleURL       string
	Config           ECSTaskConfig
}

type ECSLogEvent struct {
	Timestamp time.Time
	Message   string
}

type ECSLogStream struct {
	Container string
	LogGroup  string
	LogStream string
	Events    []ECSLogEvent
}

type ECSTaskConfig struct {
	OperatingSystem   string
	Architecture      string
	CPU               string
	Memory            string
	PlatformVersion   string
	TaskExecutionRole string
	TaskRole          string
	FaultInjection    string
	ECSExec           string
	CapacityProvider  string
	LaunchType        string
	TaskDefinition    string
	TaskGroup         string
	ServiceName       string
	ENIID             string
	NetworkMode       string
	SubnetID          string
	PublicIP          string
	PrivateIP         string
	MACAddress        string
}

type ECSLoadBalancer struct {
	Type             string
	Name             string
	ContainerMapping string
	Listener         string
	TargetGroup      string
	TargetGroupArn   string
}

type ECSClusterData struct {
	Services []ECSService
	Tags     map[string]string
}

type ECSContainerInstance struct {
	Ec2InstanceID     string
	Status            string // ACTIVE, DRAINING, REGISTERING, ...
	AgentConnected    bool
	AgentVersion      string
	RunningTasksCount int32
	PendingTasksCount int32
}

type ECSTaskDefinitionRevision struct {
	Arn      string
	Revision int32
}

type ECSTaskDefinitionContainer struct {
	Name        string
	Image       string
	CPU         int32
	Memory      int32
	Environment map[string]string
}

type ECSTaskDefinitionDetail struct {
	Family     string
	Revision   int32
	CPU        string
	Memory     string
	Containers []ECSTaskDefinitionContainer
}

func (c *Client) ListECSClusters(ctx context.Context) ([]ECSCluster, error) {
	if c.ECS == nil {
		return nil, fmt.Errorf("ECS client not initialized")
	}

	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()

	var clusters []ECSCluster
	var nextToken *string
	for {
		out, err := c.ECS.ListClusters(timeoutCtx, &ecs.ListClustersInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list ECS clusters: %w", err)
		}
		if len(out.ClusterArns) == 0 {
			break
		}

		descOut, err := c.ECS.DescribeClusters(timeoutCtx, &ecs.DescribeClustersInput{
			Clusters: out.ClusterArns,
			// Both fields ride the call that already runs; asking for them separately would be a second describe per page for data the same response can carry.
			Include: []ecsTypes.ClusterField{ecsTypes.ClusterFieldStatistics, ecsTypes.ClusterFieldSettings},
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
			if c.Region != "" && c.AccountID != "" {
				cluster.ConsoleURL = fmt.Sprintf("https://%s.console.aws.amazon.com/ecs/v2/clusters/%s?region=%s", c.Region, cluster.Name, c.Region)
			}

			cluster.CapacityProviders = cl.CapacityProviders
			cluster.Statistics = mapClusterStatistics(cl.Statistics)
			cluster.ContainerInsights = containerInsightsSetting(cl.Settings)
			for _, s := range cl.DefaultCapacityProviderStrategy {
				cluster.DefaultCapacityProviderStrat = append(cluster.DefaultCapacityProviderStrat, ECSCapacityProviderStrategy{
					CapacityProvider: getString(s.CapacityProvider),
					Weight:           s.Weight,
					Base:             s.Base,
				})
			}

			clusters = append(clusters, cluster)
		}

		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	c.recordClusterInsights(clusters)
	return clusters, nil
}

// mapClusterStatistics reads the STATISTICS key-value list into named fields.
// Keys are matched case-insensitively because AWS documents them with a leading capital while the API answers with a leading lowercase, and a key that misses is silently zero rather than an error.
func mapClusterStatistics(stats []ecsTypes.KeyValuePair) ECSClusterStatistics {
	byName := make(map[string]int32, len(stats))
	for _, kv := range stats {
		n, err := strconv.ParseInt(getString(kv.Value), 10, 32)
		if err != nil {
			continue
		}
		byName[strings.ToLower(getString(kv.Name))] = int32(n)
	}
	return ECSClusterStatistics{
		RunningEC2Tasks:         byName["runningec2taskscount"],
		RunningFargateTasks:     byName["runningfargatetaskscount"],
		PendingEC2Tasks:         byName["pendingec2taskscount"],
		PendingFargateTasks:     byName["pendingfargatetaskscount"],
		ActiveEC2Services:       byName["activeec2servicecount"],
		ActiveFargateServices:   byName["activefargateservicecount"],
		DrainingEC2Services:     byName["drainingec2servicecount"],
		DrainingFargateServices: byName["drainingfargateservicecount"],
	}
}

func (c *Client) clusterInsightsSetting(clusterName string) string {
	c.clusterInsightsMu.Lock()
	defer c.clusterInsightsMu.Unlock()
	return c.clusterInsights[clusterName]
}

func (c *Client) recordClusterInsights(clusters []ECSCluster) {
	c.clusterInsightsMu.Lock()
	defer c.clusterInsightsMu.Unlock()
	if c.clusterInsights == nil {
		c.clusterInsights = map[string]string{}
	}
	for _, cl := range clusters {
		c.clusterInsights[cl.Name] = cl.ContainerInsights
	}
}

func containerInsightsSetting(settings []ecsTypes.ClusterSetting) string {
	for _, s := range settings {
		if s.Name == ecsTypes.ClusterSettingNameContainerInsights {
			return getString(s.Value)
		}
	}
	return ""
}

// ContainerInsightsEnabled reports whether the Insights metric namespace is worth querying for this cluster.
// enhanced is the observability tier above enabled, not a different answer to "does ECS/ContainerInsights publish here".
func ContainerInsightsEnabled(setting string) bool {
	switch strings.ToLower(setting) {
	case "enabled", "enhanced":
		return true
	default:
		return false
	}
}

func (c *Client) ListECSServices(ctx context.Context, clusterName string) ([]ECSService, error) {
	if c.ECS == nil {
		return nil, fmt.Errorf("ECS client not initialized")
	}

	timeoutCtx, cancel := withDefaultTimeout(ctx, 20*time.Second)
	defer cancel()

	var services []ECSService
	var nextToken *string
	for {
		listOut, err := c.ECS.ListServices(timeoutCtx, &ecs.ListServicesInput{
			Cluster:   &clusterName,
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list ECS services: %w", err)
		}
		if len(listOut.ServiceArns) == 0 {
			break
		}

		descOut, err := c.ECS.DescribeServices(timeoutCtx, &ecs.DescribeServicesInput{
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
				Cluster:                    clusterName,
				Region:                     c.Region,
				CreatedAt:                  svc.CreatedAt,
				DesiredCount:               svc.DesiredCount,
				RunningCount:               svc.RunningCount,
				PendingCount:               svc.PendingCount,
				LaunchType:                 string(svc.LaunchType),
				HealthCheckGracePeriodSecs: getInt32Value(svc.HealthCheckGracePeriodSeconds),
			}

			if svc.DeploymentController != nil {
				service.DeploymentController = string(svc.DeploymentController.Type)
			}
			if svc.DeploymentConfiguration != nil && svc.DeploymentConfiguration.DeploymentCircuitBreaker != nil {
				service.CircuitBreakerEnabled = svc.DeploymentConfiguration.DeploymentCircuitBreaker.Enable
				service.CircuitBreakerRollback = svc.DeploymentConfiguration.DeploymentCircuitBreaker.Rollback
			}

			for _, dep := range svc.Deployments {
				service.Deployments = append(service.Deployments, ECSDeployment{
					Status:  getString(dep.Status),
					Desired: dep.DesiredCount,
					Running: dep.RunningCount,
					Pending: dep.PendingCount,
					Created: dep.CreatedAt,
				})
			}

			for _, lb := range svc.LoadBalancers {
				service.LoadBalancers = append(service.LoadBalancers, ECSLoadBalancer{
					Type:             inferLBType(lb),
					Name:             extractLBName(lb),
					ContainerMapping: fmt.Sprintf("%s:%d", getString(lb.ContainerName), getInt32Value(lb.ContainerPort)),
					Listener:         "", // not available without elbv2 call
					TargetGroup:      extractTargetGroup(lb),
					TargetGroupArn:   getString(lb.TargetGroupArn),
				})
			}

			if c.AccountID != "" && c.Region != "" {
				service.ConsoleURL = fmt.Sprintf("https://%s.console.aws.amazon.com/ecs/v2/clusters/%s/services/%s/health?region=%s", c.Region, clusterName, service.Name, c.Region)
			}

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

func (c *Client) ListECSTasks(ctx context.Context, clusterName, serviceName string) ([]ECSTask, error) {
	if c.ECS == nil {
		return nil, fmt.Errorf("ECS client not initialized")
	}

	timeoutCtx, cancel := withDefaultTimeout(ctx, 20*time.Second)
	defer cancel()

	var taskArns []string
	var nextToken *string
	for {
		listOut, err := c.ECS.ListTasks(timeoutCtx, &ecs.ListTasksInput{
			Cluster:     &clusterName,
			ServiceName: &serviceName,
			NextToken:   nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list ECS tasks: %w", err)
		}
		taskArns = append(taskArns, listOut.TaskArns...)
		if listOut.NextToken == nil {
			break
		}
		nextToken = listOut.NextToken
	}
	if len(taskArns) == 0 {
		return nil, nil
	}

	taskDefs := make(map[string]*ecsTypes.TaskDefinition)
	getTaskDef := func(arn string) (*ecsTypes.TaskDefinition, error) {
		if td, ok := taskDefs[arn]; ok {
			return td, nil
		}
		out, err := c.ECS.DescribeTaskDefinition(timeoutCtx, &ecs.DescribeTaskDefinitionInput{
			TaskDefinition: sdkaws.String(arn),
		})
		if err != nil {
			return nil, err
		}
		taskDefs[arn] = out.TaskDefinition
		return out.TaskDefinition, nil
	}

	// DescribeTasks accepts at most 100 ARNs per call.
	var tasks []ECSTask
	for _, group := range chunkStrings(taskArns, 100) {
		descOut, err := c.ECS.DescribeTasks(timeoutCtx, &ecs.DescribeTasksInput{
			Cluster: &clusterName,
			Tasks:   group,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to describe ECS tasks: %w", err)
		}
		for _, t := range descOut.Tasks {
			tasks = append(tasks, c.buildECSTask(t, clusterName, serviceName, getTaskDef))
		}
	}

	return tasks, nil
}

func chunkStrings(items []string, size int) [][]string {
	if size <= 0 || len(items) == 0 {
		return nil
	}
	var chunks [][]string
	for start := 0; start < len(items); start += size {
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[start:end])
	}
	return chunks
}

func (c *Client) buildECSTask(t ecsTypes.Task, clusterName, serviceName string, getTaskDef func(string) (*ecsTypes.TaskDefinition, error)) ECSTask {
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

	if c.Region != "" && c.AccountID != "" {
		task.ConsoleURL = fmt.Sprintf("https://%s.console.aws.amazon.com/ecs/v2/clusters/%s/tasks/%s/configuration?region=%s", c.Region, clusterName, task.ID, c.Region)
	}

	tdArn := getString(t.TaskDefinitionArn)
	td, tdErr := getTaskDef(tdArn)
	if tdErr == nil && td != nil {
		cdByName := map[string]ecsTypes.ContainerDefinition{}
		for _, cd := range td.ContainerDefinitions {
			cdByName[getString(cd.Name)] = cd
		}

		var osFam, arch string
		if td.RuntimePlatform != nil {
			osFam = string(td.RuntimePlatform.OperatingSystemFamily)
			arch = string(td.RuntimePlatform.CpuArchitecture)
		}

		task.Config = ECSTaskConfig{
			OperatingSystem:   osFam,
			Architecture:      arch,
			CPU:               formatCPUString(getString(td.Cpu)),
			Memory:            formatMemoryString(getString(td.Memory)),
			PlatformVersion:   getString(t.PlatformVersion),
			TaskExecutionRole: trimRoleName(getString(td.ExecutionRoleArn)),
			TaskRole:          trimRoleName(getString(td.TaskRoleArn)),
			FaultInjection:    "-",
			ECSExec:           formatBoolValue(t.EnableExecuteCommand),
			CapacityProvider:  getString(t.CapacityProviderName),
			LaunchType:        string(t.LaunchType),
			TaskDefinition:    tdArn,
			TaskGroup:         getString(t.Group),
			ServiceName:       serviceName,
			NetworkMode:       string(td.NetworkMode),
		}

		for _, att := range t.Attachments {
			for _, d := range att.Details {
				switch getString(d.Name) {
				case "networkInterfaceId":
					task.Config.ENIID = getString(d.Value)
				case "subnetId":
					task.Config.SubnetID = getString(d.Value)
				case "privateIPv4Address":
					task.Config.PrivateIP = getString(d.Value)
				case "publicIPv4Address":
					task.Config.PublicIP = getString(d.Value)
				case "macAddress":
					task.Config.MACAddress = getString(d.Value)
				}
			}
		}

		for _, ctn := range t.Containers {
			container := ECSContainer{
				Name:         getString(ctn.Name),
				LastStatus:   getString(ctn.LastStatus),
				HealthStatus: string(ctn.HealthStatus),
				ImageURI:     getString(ctn.Image),
				ImageDigest:  getString(ctn.ImageDigest),
				RuntimeID:    getString(ctn.RuntimeId),
			}
			if cd, ok := cdByName[getString(ctn.Name)]; ok {
				if cd.Cpu > 0 {
					container.CPU = float64(cd.Cpu) / 1024.0
				}
				container.MemoryHardMB = getInt32Value(cd.Memory)
				container.MemorySoftMB = getInt32Value(cd.MemoryReservation)
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
	} else {
		for _, ctn := range t.Containers {
			container := ECSContainer{
				Name:         getString(ctn.Name),
				LastStatus:   getString(ctn.LastStatus),
				HealthStatus: string(ctn.HealthStatus),
				ImageURI:     getString(ctn.Image),
				ImageDigest:  getString(ctn.ImageDigest),
				RuntimeID:    getString(ctn.RuntimeId),
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
	}

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

	return task
}

// ExecECSTask returns an unstarted command so the caller can attach stdio while the TUI is suspended.
func (c *Client) ExecECSTask(clusterName, taskArn, containerName string) *exec.Cmd {
	return exec.Command("aws", "ecs", "execute-command",
		"--cluster", clusterName,
		"--task", taskArn,
		"--container", containerName,
		"--command", "/bin/sh",
		"--interactive",
		"--region", c.Region,
	)
}

func (c *Client) GetECSTaskLogs(ctx context.Context, clusterName, taskArn string, limit int32) ([]ECSLogStream, error) {
	if c.ECS == nil || c.CloudWatchLogs == nil {
		return nil, fmt.Errorf("ECS or CloudWatchLogs client not initialized")
	}

	if limit <= 0 {
		limit = 50
	}

	timeoutCtx, cancel := withDefaultTimeout(ctx, 20*time.Second)
	defer cancel()

	taskID := extractTaskID(taskArn)

	taskDesc, err := c.ECS.DescribeTasks(timeoutCtx, &ecs.DescribeTasksInput{
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

	tdOut, err := c.ECS.DescribeTaskDefinition(timeoutCtx, &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: task.TaskDefinitionArn,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe task definition for logs: %w", err)
	}

	// Per-container best-effort: collect failures and keep going so one broken container does not hide the others' logs.
	var streams []ECSLogStream
	var errs []error
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

		events, err := c.fetchLogEvents(timeoutCtx, logGroup, logStream, limit)
		if err != nil {
			errs = append(errs, fmt.Errorf("container %s (%s/%s): %w", getString(cd.Name), logGroup, logStream, err))
			continue
		}

		streams = append(streams, ECSLogStream{
			Container: getString(cd.Name),
			LogGroup:  logGroup,
			LogStream: logStream,
			Events:    events,
		})
	}

	// The UI treats a non-nil error as total failure, so partial success must return nil error.
	if len(streams) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return streams, nil
}

// LoadECSClusterData preserves partial data and joins errors from failed sections.
func (c *Client) LoadECSClusterData(ctx context.Context, clusterName string) (*ECSClusterData, error) {
	if c.ECS == nil {
		return nil, fmt.Errorf("ECS client not initialized")
	}

	data := &ECSClusterData{
		Tags: map[string]string{},
	}
	var errs []error

	services, err := c.ListECSServices(ctx, clusterName)
	if err != nil {
		errs = append(errs, err)
	} else {
		data.Services = services
	}

	tags, err := c.listClusterTags(ctx, clusterName)
	if err != nil {
		errs = append(errs, err)
	} else {
		data.Tags = tags
	}

	return data, errors.Join(errs...)
}

func (c *Client) listClusterTags(ctx context.Context, clusterName string) (map[string]string, error) {
	if c.ECS == nil {
		return nil, fmt.Errorf("ECS client not initialized")
	}
	out, err := c.ECS.DescribeClusters(ctx, &ecs.DescribeClustersInput{
		Clusters: []string{clusterName},
		Include:  []ecsTypes.ClusterField{ecsTypes.ClusterFieldTags},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Clusters) == 0 {
		return map[string]string{}, nil
	}
	tags := map[string]string{}
	for _, t := range out.Clusters[0].Tags {
		if t.Key != nil && t.Value != nil {
			tags[*t.Key] = *t.Value
		}
	}
	return tags, nil
}

// ListContainerInstances returns nil, nil for Fargate-only clusters; the API legitimately returns zero ARNs there, not an error.
func (c *Client) ListContainerInstances(ctx context.Context, clusterName string) ([]ECSContainerInstance, error) {
	if c.ECS == nil {
		return nil, fmt.Errorf("ECS client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()

	var arns []string
	var nextToken *string
	for {
		out, err := c.ECS.ListContainerInstances(timeoutCtx, &ecs.ListContainerInstancesInput{
			Cluster:   &clusterName,
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list container instances: %w", err)
		}
		arns = append(arns, out.ContainerInstanceArns...)
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	if len(arns) == 0 {
		return nil, nil
	}

	var instances []ECSContainerInstance
	for _, group := range chunkStrings(arns, 100) {
		descOut, err := c.ECS.DescribeContainerInstances(timeoutCtx, &ecs.DescribeContainerInstancesInput{
			Cluster:            &clusterName,
			ContainerInstances: group,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to describe container instances: %w", err)
		}
		for _, ci := range descOut.ContainerInstances {
			instance := ECSContainerInstance{
				Ec2InstanceID:     getString(ci.Ec2InstanceId),
				Status:            getString(ci.Status),
				AgentConnected:    ci.AgentConnected,
				RunningTasksCount: ci.RunningTasksCount,
				PendingTasksCount: ci.PendingTasksCount,
			}
			if ci.VersionInfo != nil {
				instance.AgentVersion = getString(ci.VersionInfo.AgentVersion)
			}
			instances = append(instances, instance)
		}
	}

	return instances, nil
}

func (c *Client) ListTaskDefinitionFamilies(ctx context.Context) ([]string, error) {
	if c.ECS == nil {
		return nil, fmt.Errorf("ECS client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()

	var families []string
	var nextToken *string
	for {
		out, err := c.ECS.ListTaskDefinitionFamilies(timeoutCtx, &ecs.ListTaskDefinitionFamiliesInput{NextToken: nextToken})
		if err != nil {
			return nil, fmt.Errorf("failed to list task definition families: %w", err)
		}
		families = append(families, out.Families...)
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	return families, nil
}

// ListTaskDefinitions parses ARN suffixes to avoid one describe call per revision.
func (c *Client) ListTaskDefinitions(ctx context.Context, family string) ([]ECSTaskDefinitionRevision, error) {
	if c.ECS == nil {
		return nil, fmt.Errorf("ECS client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()

	var arns []string
	var nextToken *string
	for {
		out, err := c.ECS.ListTaskDefinitions(timeoutCtx, &ecs.ListTaskDefinitionsInput{
			FamilyPrefix: &family,
			Sort:         ecsTypes.SortOrderDesc,
			NextToken:    nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list task definitions for family %s: %w", family, err)
		}
		arns = append(arns, out.TaskDefinitionArns...)
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	revisions := make([]ECSTaskDefinitionRevision, len(arns))
	for i, arn := range arns {
		revisions[i] = ECSTaskDefinitionRevision{Arn: arn, Revision: extractTaskDefRevision(arn)}
	}
	return revisions, nil
}

func (c *Client) DescribeTaskDefinitionDetail(ctx context.Context, taskDefArn string) (*ECSTaskDefinitionDetail, error) {
	if c.ECS == nil {
		return nil, fmt.Errorf("ECS client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 15*time.Second)
	defer cancel()

	out, err := c.ECS.DescribeTaskDefinition(timeoutCtx, &ecs.DescribeTaskDefinitionInput{TaskDefinition: &taskDefArn})
	if err != nil {
		return nil, fmt.Errorf("failed to describe task definition %s: %w", taskDefArn, err)
	}
	td := out.TaskDefinition

	detail := &ECSTaskDefinitionDetail{
		Family:   getString(td.Family),
		Revision: td.Revision,
		CPU:      getString(td.Cpu),
		Memory:   getString(td.Memory),
	}
	for _, cd := range td.ContainerDefinitions {
		env := make(map[string]string, len(cd.Environment))
		for _, kv := range cd.Environment {
			env[getString(kv.Name)] = getString(kv.Value)
		}
		detail.Containers = append(detail.Containers, ECSTaskDefinitionContainer{
			Name:        getString(cd.Name),
			Image:       getString(cd.Image),
			CPU:         cd.Cpu,
			Memory:      getInt32Value(cd.Memory),
			Environment: env,
		})
	}
	return detail, nil
}

func TaskDefinitionFamily(taskDefArn string) string {
	rest := taskDefArn
	if idx := strings.LastIndex(taskDefArn, "/"); idx != -1 {
		rest = taskDefArn[idx+1:]
	}
	if idx := strings.LastIndex(rest, ":"); idx != -1 {
		return rest[:idx]
	}
	return rest
}

func extractTaskDefRevision(arn string) int32 {
	idx := strings.LastIndex(arn, ":")
	if idx == -1 {
		return 0
	}
	v, err := strconv.Atoi(arn[idx+1:])
	if err != nil {
		return 0
	}
	return int32(v)
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

func extractTaskID(taskArn string) string {
	parts := strings.Split(taskArn, "/")
	if len(parts) == 0 {
		return taskArn
	}
	return parts[len(parts)-1]
}

func trimRoleName(arn string) string {
	if arn == "" {
		return ""
	}
	parts := strings.Split(arn, "/")
	return parts[len(parts)-1]
}

func formatBoolValue(v bool) string {
	if v {
		return "Enabled"
	}
	return "Disabled"
}

func formatCPUString(raw string) string {
	if raw == "" {
		return ""
	}
	// AWS uses CPU units where 1024 == 1 vCPU.
	val, err := strconv.Atoi(raw)
	if err != nil {
		return raw
	}
	return fmt.Sprintf("%.3f vCPU", float64(val)/1024.0)
}

func formatMemoryString(raw string) string {
	if raw == "" {
		return ""
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val == 0 {
		return raw
	}
	gb := float64(val) / 1024.0
	return fmt.Sprintf("%.3f GB", gb)
}

func getInt32Value(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
}

func extractTargetGroup(lb ecsTypes.LoadBalancer) string {
	if lb.TargetGroupArn != nil {
		parts := strings.Split(*lb.TargetGroupArn, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	return getString(lb.TargetGroupArn)
}

func extractLBName(lb ecsTypes.LoadBalancer) string {
	if lb.LoadBalancerName != nil {
		return *lb.LoadBalancerName
	}
	return extractTargetGroup(lb)
}

func inferLBType(lb ecsTypes.LoadBalancer) string {
	// Avoid an extra load-balancer call; ambiguous names remain generic.
	if lb.LoadBalancerName != nil && strings.Contains(strings.ToLower(*lb.LoadBalancerName), "nlb") {
		return "Network Load Balancer"
	}
	return "Load Balancer"
}
