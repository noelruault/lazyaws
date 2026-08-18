package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type InstanceMetrics struct {
	InstanceID        string
	CPUUtilization    float64
	NetworkIn         float64
	NetworkOut        float64
	DiskReadBytes     float64
	DiskWriteBytes    float64
	StatusCheckFailed int
	Period            string
}

func (c *Client) GetInstanceMetrics(ctx context.Context, instanceID string) (*InstanceMetrics, error) {
	metrics := &InstanceMetrics{
		InstanceID: instanceID,
		Period:     "Last 5 minutes",
	}

	endTime := time.Now()
	startTime := endTime.Add(-5 * time.Minute)

	getMetric := func(metricName string, stat string) (float64, error) {
		namespace := "AWS/EC2"
		dimensionName := "InstanceId"
		input := &cloudwatch.GetMetricStatisticsInput{
			Namespace:  &namespace,
			MetricName: &metricName,
			Dimensions: []types.Dimension{
				{
					Name:  &dimensionName,
					Value: &instanceID,
				},
			},
			StartTime:  &startTime,
			EndTime:    &endTime,
			Period:     getInt32Ptr(300),
			Statistics: []types.Statistic{types.Statistic(stat)},
		}

		result, err := c.CloudWatch.GetMetricStatistics(ctx, input)
		if err != nil {
			return 0, err
		}

		if len(result.Datapoints) == 0 {
			return 0, nil
		}

		var latestDatapoint types.Datapoint
		var latestTime time.Time
		for _, dp := range result.Datapoints {
			if dp.Timestamp != nil && dp.Timestamp.After(latestTime) {
				latestTime = *dp.Timestamp
				latestDatapoint = dp
			}
		}

		switch stat {
		case "Average":
			if latestDatapoint.Average != nil {
				return *latestDatapoint.Average, nil
			}
		case "Sum":
			if latestDatapoint.Sum != nil {
				return *latestDatapoint.Sum, nil
			}
		case "Maximum":
			if latestDatapoint.Maximum != nil {
				return *latestDatapoint.Maximum, nil
			}
		}

		return 0, nil
	}

	cpu, err := getMetric("CPUUtilization", "Average")
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU metrics: %w", err)
	}
	metrics.CPUUtilization = cpu

	networkIn, err := getMetric("NetworkIn", "Sum")
	if err != nil {
		return nil, fmt.Errorf("failed to get network in metrics: %w", err)
	}
	metrics.NetworkIn = networkIn

	networkOut, err := getMetric("NetworkOut", "Sum")
	if err != nil {
		return nil, fmt.Errorf("failed to get network out metrics: %w", err)
	}
	metrics.NetworkOut = networkOut

	diskRead, err := getMetric("DiskReadBytes", "Sum")
	if err != nil {
		return nil, fmt.Errorf("failed to get disk read metrics: %w", err)
	}
	metrics.DiskReadBytes = diskRead

	diskWrite, err := getMetric("DiskWriteBytes", "Sum")
	if err != nil {
		return nil, fmt.Errorf("failed to get disk write metrics: %w", err)
	}
	metrics.DiskWriteBytes = diskWrite

	statusCheck, err := getMetric("StatusCheckFailed", "Maximum")
	if err != nil {
		return nil, fmt.Errorf("failed to get status check metrics: %w", err)
	}
	metrics.StatusCheckFailed = int(statusCheck)

	return metrics, nil
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
