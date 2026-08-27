package aws

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

func TestComputeUtilizationPercent(t *testing.T) {
	cases := []struct {
		name               string
		utilized, reserved float64
		want               float64
	}{
		{"half used", 512, 1024, 50},
		{"no reservation", 512, 0, 0},
		{"fully used", 1024, 1024, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := computeUtilizationPercent(tc.utilized, tc.reserved); got != tc.want {
				t.Errorf("computeUtilizationPercent(%v, %v) = %v, want %v", tc.utilized, tc.reserved, got, tc.want)
			}
		})
	}
}

func metricResult(id string, points ...MetricPoint) types.MetricDataResult {
	result := types.MetricDataResult{Id: &id}
	for _, p := range points {
		result.Timestamps = append(result.Timestamps, p.At)
		result.Values = append(result.Values, p.Value)
	}
	return result
}

func TestLatestPoint(t *testing.T) {
	at := func(min int) time.Time { return time.Date(2026, 8, 27, 17, min, 0, 0, time.UTC) }

	cases := []struct {
		name   string
		result types.MetricDataResult
		want   MetricPoint
	}{
		{"empty series is absent, not zero", metricResult("diskr"), MetricPoint{}},
		{"single series", metricResult("cpu", MetricPoint{Value: 0.523, At: at(43)}), MetricPoint{Value: 0.523, At: at(43), OK: true}},
		{
			"unsorted timestamps pick the newest",
			metricResult("cpu",
				MetricPoint{Value: 1, At: at(33)},
				MetricPoint{Value: 3, At: at(43)},
				MetricPoint{Value: 2, At: at(38)},
			),
			MetricPoint{Value: 3, At: at(43), OK: true},
		},
		{
			"newest-first ordering is not assumed",
			metricResult("cpu",
				MetricPoint{Value: 3, At: at(43)},
				MetricPoint{Value: 1, At: at(33)},
			),
			MetricPoint{Value: 3, At: at(43), OK: true},
		},
		{"a zero reading is a reading", metricResult("cpu", MetricPoint{Value: 0, At: at(43)}), MetricPoint{Value: 0, At: at(43), OK: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := latestPoint(tc.result); got != tc.want {
				t.Errorf("latestPoint() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// A short Values slice would panic an index-paired read; CloudWatch pairs them, but nothing in the type system says so.
func TestLatestPointToleratesUnpairedSlices(t *testing.T) {
	at := time.Date(2026, 8, 27, 17, 43, 0, 0, time.UTC)
	result := metricResult("cpu")
	result.Timestamps = []time.Time{at, at.Add(time.Minute)}
	result.Values = []float64{7}

	want := MetricPoint{Value: 7, At: at, OK: true}
	if got := latestPoint(result); got != want {
		t.Errorf("latestPoint() = %+v, want %+v", got, want)
	}
}

func TestMapInstanceMetricsIgnoresResponseOrder(t *testing.T) {
	at := time.Date(2026, 8, 27, 17, 43, 0, 0, time.UTC)
	// Deliberately not the order instanceMetricQueries asks in, and disk read is the empty series an EBS-only instance returns.
	results := []types.MetricDataResult{
		metricResult(metricIDStatusCheckFailed, MetricPoint{Value: 0, At: at}),
		metricResult(metricIDNetworkOut, MetricPoint{Value: 305000, At: at}),
		metricResult(metricIDCPU, MetricPoint{Value: 0.523, At: at}),
		metricResult(metricIDDiskRead),
		metricResult(metricIDNetworkIn, MetricPoint{Value: 235000, At: at}),
		metricResult(metricIDDiskWrite, MetricPoint{Value: 4096, At: at}),
	}

	got := mapInstanceMetrics("i-0abcdef1234567890", results)

	if got.InstanceID != "i-0abcdef1234567890" {
		t.Errorf("InstanceID = %q, want the requested instance", got.InstanceID)
	}
	if got.CPUUtilization != (MetricPoint{Value: 0.523, At: at, OK: true}) {
		t.Errorf("CPUUtilization = %+v, want the cpu result", got.CPUUtilization)
	}
	if got.NetworkIn.Value != 235000 || got.NetworkOut.Value != 305000 {
		t.Errorf("network in/out = %v/%v, want 235000/305000 (results matched by id, not position)", got.NetworkIn.Value, got.NetworkOut.Value)
	}
	if got.DiskWriteBytes.Value != 4096 || !got.DiskWriteBytes.OK {
		t.Errorf("DiskWriteBytes = %+v, want 4096 present", got.DiskWriteBytes)
	}
	if got.DiskReadBytes.OK {
		t.Errorf("DiskReadBytes = %+v, want absent: an empty series must never read as 0", got.DiskReadBytes)
	}
	if !got.StatusCheckFailed.OK || got.StatusCheckFailed.Value != 0 {
		t.Errorf("StatusCheckFailed = %+v, want a present zero (a passing check is data)", got.StatusCheckFailed)
	}
}

// A metric the response omits entirely must be absent, not silently mapped from a neighbour.
func TestMapInstanceMetricsLeavesUnansweredMetricsAbsent(t *testing.T) {
	got := mapInstanceMetrics("i-1234567890", nil)

	for name, point := range map[string]MetricPoint{
		"CPUUtilization":    got.CPUUtilization,
		"NetworkIn":         got.NetworkIn,
		"NetworkOut":        got.NetworkOut,
		"DiskReadBytes":     got.DiskReadBytes,
		"DiskWriteBytes":    got.DiskWriteBytes,
		"StatusCheckFailed": got.StatusCheckFailed,
	} {
		if point.OK {
			t.Errorf("%s = %+v, want absent when the response carried no result", name, point)
		}
	}
}

// The whole point of the ticket: one call, six ids, no serial GetMetricStatistics.
func TestInstanceMetricQueriesAskForEveryMetricOnce(t *testing.T) {
	queries := instanceMetricQueries("i-1234567890")

	want := map[string]string{
		metricIDCPU:               "CPUUtilization/Average",
		metricIDNetworkIn:         "NetworkIn/Sum",
		metricIDNetworkOut:        "NetworkOut/Sum",
		metricIDDiskRead:          "DiskReadBytes/Sum",
		metricIDDiskWrite:         "DiskWriteBytes/Sum",
		metricIDStatusCheckFailed: "StatusCheckFailed/Maximum",
	}
	if len(queries) != len(want) {
		t.Fatalf("got %d queries, want %d in a single call", len(queries), len(want))
	}

	seen := map[string]bool{}
	for _, q := range queries {
		id := getString(q.Id)
		if seen[id] {
			t.Errorf("query id %q repeats; GetMetricData rejects duplicate ids and results are matched by id", id)
		}
		seen[id] = true

		if q.MetricStat == nil || q.MetricStat.Metric == nil {
			t.Fatalf("query %q has no MetricStat", id)
		}
		got := getString(q.MetricStat.Metric.MetricName) + "/" + getString(q.MetricStat.Stat)
		if got != want[id] {
			t.Errorf("query %q asks %s, want %s", id, got, want[id])
		}
		if ns := getString(q.MetricStat.Metric.Namespace); ns != "AWS/EC2" {
			t.Errorf("query %q namespace = %q, want AWS/EC2", id, ns)
		}
		// Pinned to the literal, not to metricPeriod: comparing the constant against itself passes whatever it is changed to.
		if q.MetricStat.Period == nil || *q.MetricStat.Period != 300 {
			t.Errorf("query %q period = %v, want 300 to match basic monitoring's publish interval", id, q.MetricStat.Period)
		}
		dims := q.MetricStat.Metric.Dimensions
		if len(dims) != 1 || getString(dims[0].Name) != "InstanceId" || getString(dims[0].Value) != "i-1234567890" {
			t.Errorf("query %q dimensions = %+v, want one InstanceId dimension for the requested instance", id, dims)
		}
	}
}

// The window has to span several publish periods or a live metric can answer empty: basic monitoring publishes one datapoint per period, and the freshest is already minutes old when it arrives.
func TestMetricWindowCoversSeveralPublishPeriods(t *testing.T) {
	if min := 4 * metricPeriod * time.Second; metricWindow < min {
		t.Errorf("metricWindow = %v, want at least %v (%d publish periods)", metricWindow, min, 4)
	}
}

func TestGetInstanceMetricsGuards(t *testing.T) {
	_, err := (&Client{}).GetInstanceMetrics(context.Background(), "i-1234567890")
	if err == nil {
		t.Fatal("GetInstanceMetrics() with nil CloudWatch client should error")
	}
	if !strings.Contains(err.Error(), "CloudWatch client") {
		t.Errorf("GetInstanceMetrics() nil-client error = %v, want the client guard to be what fired", err)
	}

	// A non-nil client, so only the id guard can answer: with nil the client guard fires first and hides it.
	_, err = (&Client{CloudWatch: &cloudwatch.Client{}}).GetInstanceMetrics(context.Background(), "")
	if err == nil {
		t.Fatal("GetInstanceMetrics() with empty instance id should error")
	}
	if !strings.Contains(err.Error(), "instance id required") {
		t.Errorf("GetInstanceMetrics() empty-id error = %v, want the id guard to be what fired", err)
	}
}

// The ticket's whole point: service utilization comes from AWS/ECS, which publishes whether or not Container Insights is on.
func TestServiceMetricQueriesUseThePlainECSNamespace(t *testing.T) {
	queries := serviceMetricQueries("app-cluster", "app-auth", false)

	want := map[string]string{
		metricIDECSCPU:    "CPUUtilization",
		metricIDECSMemory: "MemoryUtilization",
	}
	if len(queries) != len(want) {
		t.Fatalf("got %d queries, want %d without Insights", len(queries), len(want))
	}
	for _, q := range queries {
		id := getString(q.Id)
		if q.MetricStat == nil || q.MetricStat.Metric == nil {
			t.Fatalf("query %q has no MetricStat", id)
		}
		if got := getString(q.MetricStat.Metric.MetricName); got != want[id] {
			t.Errorf("query %q asks for %q, want %q", id, got, want[id])
		}
		// Pinned to the literal: comparing against the constant the code uses agrees with whatever the constant is changed to.
		if ns := getString(q.MetricStat.Metric.Namespace); ns != "AWS/ECS" {
			t.Errorf("query %q namespace = %q, want AWS/ECS; ECS/ContainerInsights only publishes where the setting is on", id, ns)
		}
		// The service panel captions these readings "1-min avg", so the period is what makes that caption true rather than a decoration.
		if q.MetricStat.Period == nil || *q.MetricStat.Period != 60 {
			t.Errorf("query %q period = %v, want 60 for the minute refresh tier", id, q.MetricStat.Period)
		}
		if getString(q.MetricStat.Stat) != "Average" {
			t.Errorf("query %q stat = %q, want Average: a utilization percentage does not sum", id, getString(q.MetricStat.Stat))
		}
		dims := map[string]string{}
		for _, d := range q.MetricStat.Metric.Dimensions {
			dims[getString(d.Name)] = getString(d.Value)
		}
		if len(dims) != 2 || dims["ClusterName"] != "app-cluster" || dims["ServiceName"] != "app-auth" {
			t.Errorf("query %q dimensions = %v, want exactly ClusterName+ServiceName for the requested service", id, dims)
		}
	}
}

// Insights is additive: it may add ids to the same call, and it may never replace the AWS/ECS pair or be asked for when the cluster has it off.
func TestServiceMetricQueriesAddInsightsOnlyWhenEnabled(t *testing.T) {
	namespaces := func(withInsights bool) map[string]string {
		byID := map[string]string{}
		for _, q := range serviceMetricQueries("app-cluster", "app-auth", withInsights) {
			byID[getString(q.Id)] = getString(q.MetricStat.Metric.Namespace) + "/" + getString(q.MetricStat.Metric.MetricName)
		}
		return byID
	}

	off := namespaces(false)
	for id := range off {
		if strings.Contains(off[id], "ContainerInsights") {
			t.Errorf("query %q = %s with Insights off; a cluster without Insights would be billed for an empty series every refresh", id, off[id])
		}
	}

	on := namespaces(true)
	if on[metricIDECSCPU] != "AWS/ECS/CPUUtilization" || on[metricIDECSMemory] != "AWS/ECS/MemoryUtilization" {
		t.Errorf("with Insights on the AWS/ECS pair = %q/%q, want it kept as the source rather than replaced", on[metricIDECSCPU], on[metricIDECSMemory])
	}
	for id, want := range map[string]string{
		metricIDECSCPUUsed:     "ECS/ContainerInsights/CpuUtilized",
		metricIDECSCPUReserved: "ECS/ContainerInsights/CpuReserved",
		metricIDECSMemUsed:     "ECS/ContainerInsights/MemoryUtilized",
		metricIDECSMemReserved: "ECS/ContainerInsights/MemoryReserved",
	} {
		if on[id] != want {
			t.Errorf("query %q = %q, want %q added when the cluster setting is on", id, on[id], want)
		}
	}
	if len(on) != len(off)+4 {
		t.Errorf("got %d queries with Insights and %d without, want exactly four more in the same call", len(on), len(off))
	}
}

func TestMapServiceMetricsIgnoresResponseOrder(t *testing.T) {
	at := time.Date(2026, 8, 27, 17, 43, 0, 0, time.UTC)
	// Deliberately not the order serviceMetricQueries asks in, and memory is the empty series a cluster without Insights returns for the extras.
	results := []types.MetricDataResult{
		metricResult(metricIDECSMemory, MetricPoint{Value: 13.95, At: at}),
		metricResult(metricIDECSCPU, MetricPoint{Value: 1.12, At: at}),
		metricResult(metricIDECSCPUReserved),
	}

	got := mapServiceMetrics("app-cluster", "app-auth", results)

	if got.ClusterName != "app-cluster" || got.ServiceName != "app-auth" {
		t.Errorf("got %s/%s, want the requested cluster and service", got.ClusterName, got.ServiceName)
	}
	if got.CPUUtilization != (MetricPoint{Value: 1.12, At: at, OK: true}) {
		t.Errorf("CPUUtilization = %+v, want the ecscpu result matched by id, not by position", got.CPUUtilization)
	}
	if got.MemoryUtilization.Value != 13.95 || !got.MemoryUtilization.OK {
		t.Errorf("MemoryUtilization = %+v, want 13.95 present", got.MemoryUtilization)
	}
	if got.InsightsCPUTotal.OK {
		t.Errorf("InsightsCPUTotal = %+v, want absent: an empty series must never read as a reservation of 0", got.InsightsCPUTotal)
	}
	if got.InsightsMemTotal.OK || got.InsightsMemUsed.OK || got.InsightsCPUUsed.OK {
		t.Error("unanswered Insights metrics must stay absent rather than being mapped from a neighbour")
	}
}

// A zero-percent service is idle, not unmeasured, and the two must not render the same.
func TestMapServiceMetricsKeepsAMeasuredZero(t *testing.T) {
	at := time.Date(2026, 8, 27, 17, 43, 0, 0, time.UTC)
	got := mapServiceMetrics("c", "s", []types.MetricDataResult{metricResult(metricIDECSCPU, MetricPoint{Value: 0, At: at})})

	if !got.CPUUtilization.OK || got.CPUUtilization.Value != 0 {
		t.Errorf("CPUUtilization = %+v, want a present zero", got.CPUUtilization)
	}
	if got.MemoryUtilization.OK {
		t.Errorf("MemoryUtilization = %+v, want absent when the response carried no result", got.MemoryUtilization)
	}
}

func TestGetECSServiceMetricsGuards(t *testing.T) {
	_, err := (&Client{}).GetECSServiceMetrics(context.Background(), "c", "s")
	if err == nil || !strings.Contains(err.Error(), "CloudWatch client") {
		t.Errorf("nil-client error = %v, want the client guard to be what fired", err)
	}

	// A non-nil client, so only the name guard can answer: with nil the client guard fires first and hides it.
	for _, tc := range []struct{ cluster, service string }{{"", "s"}, {"c", ""}} {
		_, err := (&Client{CloudWatch: &cloudwatch.Client{}}).GetECSServiceMetrics(context.Background(), tc.cluster, tc.service)
		if err == nil || !strings.Contains(err.Error(), "cluster and service names required") {
			t.Errorf("GetECSServiceMetrics(%q, %q) error = %v, want the name guard to be what fired", tc.cluster, tc.service, err)
		}
	}
}

// The Insights extras are gated on what the last cluster list recorded, so the gate is what has to be pinned: nothing else tells the fetch which namespace to ask for.
func TestClusterInsightsRecordDrivesTheInsightsGate(t *testing.T) {
	c := &Client{}
	if ContainerInsightsEnabled(c.clusterInsightsSetting("app-cluster")) {
		t.Error("a cluster nobody has listed must read as Insights off, not on")
	}

	c.recordClusterInsights([]ECSCluster{
		{Name: "batch-cluster", ContainerInsights: "enabled"},
		{Name: "app-cluster", ContainerInsights: "disabled"},
	})

	if !ContainerInsightsEnabled(c.clusterInsightsSetting("batch-cluster")) {
		t.Error("batch-cluster was recorded as enabled and must gate the extras on")
	}
	if ContainerInsightsEnabled(c.clusterInsightsSetting("app-cluster")) {
		t.Error("app-cluster was recorded as disabled and must gate the extras off")
	}
}

func TestGetInstanceAlarmsGuards(t *testing.T) {
	if _, err := (&Client{}).GetInstanceAlarms(context.Background(), "i-1234567890"); err == nil {
		t.Error("GetInstanceAlarms() with nil CloudWatch client should error")
	}
	if _, err := (&Client{CloudWatch: nil}).GetInstanceAlarms(context.Background(), ""); err == nil {
		t.Error("GetInstanceAlarms() with empty instance id should error")
	}
}

func TestAlarmMatchesInstance(t *testing.T) {
	name, value := "InstanceId", "i-1234567890"
	other := "OtherDim"
	dims := []types.Dimension{{Name: &other, Value: &value}, {Name: &name, Value: &value}}

	if !alarmMatchesInstance(dims, "i-1234567890") {
		t.Error("expected match on InstanceId dimension")
	}
	if alarmMatchesInstance(dims, "i-0000000000") {
		t.Error("expected no match for a different instance id")
	}
	if alarmMatchesInstance(nil, "i-1234567890") {
		t.Error("expected no match with no dimensions")
	}
}
