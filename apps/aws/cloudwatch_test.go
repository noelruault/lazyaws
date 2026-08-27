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
