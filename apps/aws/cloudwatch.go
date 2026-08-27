package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// MetricPoint is one CloudWatch datapoint with the time CloudWatch published it.
// Absent is not zero: an EBS-only instance never publishes disk metrics, and rendering that as 0 invents a reading.
type MetricPoint struct {
	Value float64
	At    time.Time
	OK    bool
}

type InstanceMetrics struct {
	InstanceID        string
	CPUUtilization    MetricPoint
	NetworkIn         MetricPoint
	NetworkOut        MetricPoint
	DiskReadBytes     MetricPoint
	DiskWriteBytes    MetricPoint
	StatusCheckFailed MetricPoint
}

// Query ids are the only way to tell one result from another: GetMetricData documents no correspondence between request and response order.
const (
	metricIDCPU               = "cpu"
	metricIDNetworkIn         = "netin"
	metricIDNetworkOut        = "netout"
	metricIDDiskRead          = "diskr"
	metricIDDiskWrite         = "diskw"
	metricIDStatusCheckFailed = "statuscheck"

	metricIDECSCPU         = "ecscpu"
	metricIDECSMemory      = "ecsmem"
	metricIDECSCPUUsed     = "ecscpuused"
	metricIDECSCPUReserved = "ecscpureserved"
	metricIDECSMemUsed     = "ecsmemused"
	metricIDECSMemReserved = "ecsmemreserved"
)

const (
	// metricWindow spans several basic-monitoring periods so a metric that published anything recently still has a datapoint to show.
	metricWindow = 30 * time.Minute
	// metricPeriod matches basic monitoring's publish interval; asking for less returns empty buckets between datapoints, not finer data.
	metricPeriod = 300
	// ecsMetricPeriod is finer than metricPeriod because ECS service utilization is refreshed on the minute tier rather than EC2's five.
	// Asking for a period below the publish cadence only thins the series out; latestPoint reads the newest bucket that carries anything, and GetMetricData bills per metric requested, not per datapoint returned.
	ecsMetricPeriod = 60

	// ecsNamespace publishes service utilization for every cluster. ecsInsightsNamespace publishes only where Container Insights is switched on, which is why it is an extra and never the source.
	ecsNamespace         = "AWS/ECS"
	ecsInsightsNamespace = "ECS/ContainerInsights"
)

func instanceMetricQuery(id, metricName, stat, instanceID string) types.MetricDataQuery {
	namespace := "AWS/EC2"
	dimensionName := "InstanceId"
	return types.MetricDataQuery{
		Id: &id,
		MetricStat: &types.MetricStat{
			Metric: &types.Metric{
				Namespace:  &namespace,
				MetricName: &metricName,
				Dimensions: []types.Dimension{{Name: &dimensionName, Value: &instanceID}},
			},
			Period: getInt32Ptr(metricPeriod),
			Stat:   &stat,
		},
	}
}

// instanceMetricQueries asks for the whole EC2 metric set in one request.
// GetMetricData is billed per metric id, so six ids in one call cost what six calls cost and pay one round trip instead of six.
func instanceMetricQueries(instanceID string) []types.MetricDataQuery {
	return []types.MetricDataQuery{
		instanceMetricQuery(metricIDCPU, "CPUUtilization", "Average", instanceID),
		instanceMetricQuery(metricIDNetworkIn, "NetworkIn", "Sum", instanceID),
		instanceMetricQuery(metricIDNetworkOut, "NetworkOut", "Sum", instanceID),
		instanceMetricQuery(metricIDDiskRead, "DiskReadBytes", "Sum", instanceID),
		instanceMetricQuery(metricIDDiskWrite, "DiskWriteBytes", "Sum", instanceID),
		instanceMetricQuery(metricIDStatusCheckFailed, "StatusCheckFailed", "Maximum", instanceID),
	}
}

// latestPoint takes the newest datapoint of a result.
// Timestamps and Values are index-paired and the response carries no ordering guarantee, so the newest is searched for rather than read off either end.
func latestPoint(r types.MetricDataResult) MetricPoint {
	var point MetricPoint
	paired := len(r.Timestamps)
	if len(r.Values) < paired {
		paired = len(r.Values)
	}
	for i := 0; i < paired; i++ {
		if point.OK && !r.Timestamps[i].After(point.At) {
			continue
		}
		point = MetricPoint{Value: r.Values[i], At: r.Timestamps[i], OK: true}
	}
	return point
}

// mapInstanceMetrics indexes results by query id because GetMetricData does not answer in request order.
// A metric with nothing published comes back as an empty series rather than being omitted, and stays absent here.
func mapInstanceMetrics(instanceID string, results []types.MetricDataResult) *InstanceMetrics {
	byID := make(map[string]MetricPoint, len(results))
	for _, r := range results {
		byID[getString(r.Id)] = latestPoint(r)
	}
	return &InstanceMetrics{
		InstanceID:        instanceID,
		CPUUtilization:    byID[metricIDCPU],
		NetworkIn:         byID[metricIDNetworkIn],
		NetworkOut:        byID[metricIDNetworkOut],
		DiskReadBytes:     byID[metricIDDiskRead],
		DiskWriteBytes:    byID[metricIDDiskWrite],
		StatusCheckFailed: byID[metricIDStatusCheckFailed],
	}
}

func (c *Client) GetInstanceMetrics(ctx context.Context, instanceID string) (*InstanceMetrics, error) {
	if c.CloudWatch == nil {
		return nil, fmt.Errorf("CloudWatch client not initialized")
	}
	if instanceID == "" {
		return nil, fmt.Errorf("instance id required")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 10*time.Second)
	defer cancel()

	endTime := time.Now()
	startTime := endTime.Add(-metricWindow)

	result, err := c.CloudWatch.GetMetricData(timeoutCtx, &cloudwatch.GetMetricDataInput{
		MetricDataQueries: instanceMetricQueries(instanceID),
		StartTime:         &startTime,
		EndTime:           &endTime,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get instance metrics: %w", err)
	}

	return mapInstanceMetrics(instanceID, result.MetricDataResults), nil
}

func getInt32Ptr(i int32) *int32 {
	return &i
}

type InstanceAlarm struct {
	Name       string
	State      string // OK, ALARM, INSUFFICIENT_DATA
	MetricName string
}

// GetInstanceAlarms scans locally because DescribeAlarms cannot filter dimensions server-side.
func (c *Client) GetInstanceAlarms(ctx context.Context, instanceID string) ([]InstanceAlarm, error) {
	if c.CloudWatch == nil {
		return nil, fmt.Errorf("CloudWatch client not initialized")
	}
	if instanceID == "" {
		return nil, fmt.Errorf("instance id required")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 10*time.Second)
	defer cancel()

	var alarms []InstanceAlarm
	input := &cloudwatch.DescribeAlarmsInput{
		AlarmTypes: []types.AlarmType{types.AlarmTypeMetricAlarm},
	}
	for {
		out, err := c.CloudWatch.DescribeAlarms(timeoutCtx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to describe alarms: %w", err)
		}
		for _, a := range out.MetricAlarms {
			if !alarmMatchesInstance(a.Dimensions, instanceID) {
				continue
			}
			alarms = append(alarms, InstanceAlarm{
				Name:       getString(a.AlarmName),
				State:      string(a.StateValue),
				MetricName: getString(a.MetricName),
			})
		}
		if out.NextToken == nil {
			break
		}
		input.NextToken = out.NextToken
	}
	return alarms, nil
}

func alarmMatchesInstance(dims []types.Dimension, instanceID string) bool {
	for _, d := range dims {
		if getString(d.Name) == "InstanceId" && getString(d.Value) == instanceID {
			return true
		}
	}
	return false
}

// ECSServiceMetrics is a service's utilization as CloudWatch published it.
// CPUUtilization and MemoryUtilization come from AWS/ECS, which every cluster publishes; the Insights fields are absent unless the cluster has Container Insights on, and they carry absolute CPU units and MiB rather than a second copy of the percentages.
type ECSServiceMetrics struct {
	ClusterName       string
	ServiceName       string
	CPUUtilization    MetricPoint
	MemoryUtilization MetricPoint
	InsightsCPUUsed   MetricPoint
	InsightsCPUTotal  MetricPoint
	InsightsMemUsed   MetricPoint
	InsightsMemTotal  MetricPoint
}

// ecsMetricQuery builds one query at the ECS refresh period; the dimensions are what decide whether it asks about a whole cluster or a single service.
func ecsMetricQuery(id, namespace, metricName string, dims []types.Dimension) types.MetricDataQuery {
	stat := "Average"
	return types.MetricDataQuery{
		Id: &id,
		MetricStat: &types.MetricStat{
			Metric: &types.Metric{
				Namespace:  &namespace,
				MetricName: &metricName,
				Dimensions: dims,
			},
			Period: getInt32Ptr(ecsMetricPeriod),
			Stat:   &stat,
		},
	}
}

// ecsServiceDimensions and ecsClusterDimensions are separate rather than one variadic builder because the pair is not optional: a service metric asked for with the cluster dimension alone silently answers for the whole cluster.
func ecsServiceDimensions(clusterName, serviceName string) []types.Dimension {
	clusterDim, serviceDim := "ClusterName", "ServiceName"
	return []types.Dimension{
		{Name: &clusterDim, Value: &clusterName},
		{Name: &serviceDim, Value: &serviceName},
	}
}

func ecsClusterDimensions(clusterName string) []types.Dimension {
	clusterDim := "ClusterName"
	return []types.Dimension{{Name: &clusterDim, Value: &clusterName}}
}

// serviceMetricQueries asks AWS/ECS for the utilization every cluster publishes, and appends the Container Insights reservations only when the cluster has the setting on.
// The extras ride the same request, so having them costs four more metric ids and no extra round trip; asking for them on a cluster without Insights would bill for six empty series on every refresh.
func serviceMetricQueries(clusterName, serviceName string, withInsights bool) []types.MetricDataQuery {
	dims := ecsServiceDimensions(clusterName, serviceName)
	queries := []types.MetricDataQuery{
		ecsMetricQuery(metricIDECSCPU, ecsNamespace, "CPUUtilization", dims),
		ecsMetricQuery(metricIDECSMemory, ecsNamespace, "MemoryUtilization", dims),
	}
	if !withInsights {
		return queries
	}
	return append(queries,
		ecsMetricQuery(metricIDECSCPUUsed, ecsInsightsNamespace, "CpuUtilized", dims),
		ecsMetricQuery(metricIDECSCPUReserved, ecsInsightsNamespace, "CpuReserved", dims),
		ecsMetricQuery(metricIDECSMemUsed, ecsInsightsNamespace, "MemoryUtilized", dims),
		ecsMetricQuery(metricIDECSMemReserved, ecsInsightsNamespace, "MemoryReserved", dims),
	)
}

func mapServiceMetrics(clusterName, serviceName string, results []types.MetricDataResult) *ECSServiceMetrics {
	byID := make(map[string]MetricPoint, len(results))
	for _, r := range results {
		byID[getString(r.Id)] = latestPoint(r)
	}
	return &ECSServiceMetrics{
		ClusterName:       clusterName,
		ServiceName:       serviceName,
		CPUUtilization:    byID[metricIDECSCPU],
		MemoryUtilization: byID[metricIDECSMemory],
		InsightsCPUUsed:   byID[metricIDECSCPUUsed],
		InsightsCPUTotal:  byID[metricIDECSCPUReserved],
		InsightsMemUsed:   byID[metricIDECSMemUsed],
		InsightsMemTotal:  byID[metricIDECSMemReserved],
	}
}

// GetECSServiceMetrics reads a service's utilization from AWS/ECS, which publishes regardless of Container Insights.
// Whether to also ask for the Insights reservations is decided from the cluster setting the last cluster list recorded: a cluster nobody has listed yet reads as off, which loses an extra rather than inventing a reading.
func (c *Client) GetECSServiceMetrics(ctx context.Context, clusterName, serviceName string) (*ECSServiceMetrics, error) {
	if c.CloudWatch == nil {
		return nil, fmt.Errorf("CloudWatch client not initialized")
	}
	if clusterName == "" || serviceName == "" {
		return nil, fmt.Errorf("cluster and service names required")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 10*time.Second)
	defer cancel()

	endTime := time.Now()
	startTime := endTime.Add(-metricWindow)

	result, err := c.CloudWatch.GetMetricData(timeoutCtx, &cloudwatch.GetMetricDataInput{
		MetricDataQueries: serviceMetricQueries(clusterName, serviceName, ContainerInsightsEnabled(c.clusterInsightsSetting(clusterName))),
		StartTime:         &startTime,
		EndTime:           &endTime,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get ECS service metrics: %w", err)
	}

	return mapServiceMetrics(clusterName, serviceName, result.MetricDataResults), nil
}

// ECSClusterMetrics is a cluster's Container Insights reading in the absolute units Insights publishes, not as a percentage.
// The percentage is derived, and deriving it from two absent readings gives exactly 0.0%, which is the "idle cluster" lie an unpublished series must not be allowed to tell.
type ECSClusterMetrics struct {
	ClusterName string
	CPUUsed     MetricPoint
	CPUReserved MetricPoint
	MemUsed     MetricPoint
	MemReserved MetricPoint
}

// UtilizationPercent divides a used reading by its reservation, and reports absence unless BOTH sides are present and the reservation is above zero.
// A cluster reserving nothing and a cluster CloudWatch has not answered for both compute to 0%, and only one of them is a measurement.
func UtilizationPercent(used, reserved MetricPoint) (float64, bool) {
	if !used.OK || !reserved.OK || reserved.Value <= 0 {
		return 0, false
	}

	return used.Value / reserved.Value * 100, true
}

// clusterMetricQueries asks for a cluster's four Insights reservation series in one request.
// ECS/ContainerInsights is the namespace rather than AWS/ECS because AWS/ECS publishes CPUUtilization only for EC2 launch-type reservations, so a Fargate-only cluster reads permanently empty there.
func clusterMetricQueries(clusterName string) []types.MetricDataQuery {
	dims := ecsClusterDimensions(clusterName)
	return []types.MetricDataQuery{
		ecsMetricQuery(metricIDECSCPUUsed, ecsInsightsNamespace, "CpuUtilized", dims),
		ecsMetricQuery(metricIDECSCPUReserved, ecsInsightsNamespace, "CpuReserved", dims),
		ecsMetricQuery(metricIDECSMemUsed, ecsInsightsNamespace, "MemoryUtilized", dims),
		ecsMetricQuery(metricIDECSMemReserved, ecsInsightsNamespace, "MemoryReserved", dims),
	}
}

func mapClusterMetrics(clusterName string, results []types.MetricDataResult) *ECSClusterMetrics {
	byID := make(map[string]MetricPoint, len(results))
	for _, r := range results {
		byID[getString(r.Id)] = latestPoint(r)
	}

	return &ECSClusterMetrics{
		ClusterName: clusterName,
		CPUUsed:     byID[metricIDECSCPUUsed],
		CPUReserved: byID[metricIDECSCPUReserved],
		MemUsed:     byID[metricIDECSMemUsed],
		MemReserved: byID[metricIDECSMemReserved],
	}
}

// GetECSClusterMetrics reads a cluster's reservation utilization in ONE GetMetricData call.
// Callers are expected to have checked that Container Insights is on: this namespace publishes nothing without it, and asking anyway bills for four empty series on every refresh tick.
func (c *Client) GetECSClusterMetrics(ctx context.Context, clusterName string) (*ECSClusterMetrics, error) {
	if c.CloudWatch == nil {
		return nil, fmt.Errorf("CloudWatch client not initialized")
	}
	if clusterName == "" {
		return nil, fmt.Errorf("cluster name required")
	}

	timeoutCtx, cancel := withDefaultTimeout(ctx, 10*time.Second)
	defer cancel()

	endTime := time.Now()
	startTime := endTime.Add(-metricWindow)

	result, err := c.CloudWatch.GetMetricData(timeoutCtx, &cloudwatch.GetMetricDataInput{
		MetricDataQueries: clusterMetricQueries(clusterName),
		StartTime:         &startTime,
		EndTime:           &endTime,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get ECS cluster metrics: %w", err)
	}

	return mapClusterMetrics(clusterName, result.MetricDataResults), nil
}

type ECSContainerInsights struct {
	CPUPercent float64
	MemPercent float64
}

// computeUtilizationPercent returns 0 when reserved is unavailable (e.g. Container Insights isn't enabled on the cluster) rather than dividing by zero.
func computeUtilizationPercent(utilized, reserved float64) float64 {
	if reserved <= 0 {
		return 0
	}
	return utilized / reserved * 100
}

// GetECSContainerInsights returns zero percentages (not an error) when Container Insights isn't enabled; CloudWatch answers with an empty datapoint set, not a fault.
func (c *Client) GetECSContainerInsights(ctx context.Context, clusterName, serviceName string) (*ECSContainerInsights, error) {
	if c.CloudWatch == nil {
		return nil, fmt.Errorf("CloudWatch client not initialized")
	}
	timeoutCtx, cancel := withDefaultTimeout(ctx, 10*time.Second)
	defer cancel()

	endTime := time.Now()
	startTime := endTime.Add(-5 * time.Minute)

	dimensionName := "ClusterName"
	dims := []types.Dimension{{Name: &dimensionName, Value: &clusterName}}
	if serviceName != "" {
		serviceDim := "ServiceName"
		dims = append(dims, types.Dimension{Name: &serviceDim, Value: &serviceName})
	}

	getMetric := func(metricName string) (float64, error) {
		namespace := "ECS/ContainerInsights"
		input := &cloudwatch.GetMetricStatisticsInput{
			Namespace:  &namespace,
			MetricName: &metricName,
			Dimensions: dims,
			StartTime:  &startTime,
			EndTime:    &endTime,
			Period:     getInt32Ptr(300),
			Statistics: []types.Statistic{types.StatisticAverage},
		}
		result, err := c.CloudWatch.GetMetricStatistics(timeoutCtx, input)
		if err != nil {
			return 0, err
		}
		var latest types.Datapoint
		var latestTime time.Time
		for _, dp := range result.Datapoints {
			if dp.Timestamp != nil && dp.Timestamp.After(latestTime) {
				latestTime = *dp.Timestamp
				latest = dp
			}
		}
		if latest.Average != nil {
			return *latest.Average, nil
		}
		return 0, nil
	}

	cpuUtilized, err := getMetric("CpuUtilized")
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU utilized: %w", err)
	}
	cpuReserved, err := getMetric("CpuReserved")
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU reserved: %w", err)
	}
	memUtilized, err := getMetric("MemoryUtilized")
	if err != nil {
		return nil, fmt.Errorf("failed to get memory utilized: %w", err)
	}
	memReserved, err := getMetric("MemoryReserved")
	if err != nil {
		return nil, fmt.Errorf("failed to get memory reserved: %w", err)
	}

	return &ECSContainerInsights{
		CPUPercent: computeUtilizationPercent(cpuUtilized, cpuReserved),
		MemPercent: computeUtilizationPercent(memUtilized, memReserved),
	}, nil
}
