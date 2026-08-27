package aws

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMetricsMemoFresh(t *testing.T) {
	taken := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		ask     string
		maxAge  time.Duration
		now     time.Time
		wantHit bool
	}{
		{name: "within the tier", ask: "i-1", maxAge: time.Minute, now: taken.Add(59 * time.Second), wantHit: true},
		{name: "exactly the tier's age is already out of date", ask: "i-1", maxAge: time.Minute, now: taken.Add(time.Minute)},
		{name: "past the tier", ask: "i-1", maxAge: time.Minute, now: taken.Add(90 * time.Second)},
		{name: "another resource never answers from this one", ask: "i-2", maxAge: time.Minute, now: taken, wantHit: false},
		// 0 is what MetricsInterval reports with the metrics tier switched off, and it must mean "do not refetch", not "refetch every time".
		{name: "no tier means the reading never goes out of date", ask: "i-1", maxAge: 0, now: taken.Add(72 * time.Hour), wantHit: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memo := &metricsMemo[*InstanceMetrics]{}
			memo.keep("i-1", &InstanceMetrics{InstanceID: "i-1"}, taken)

			got, ok := memo.fresh(tt.ask, tt.maxAge, tt.now)
			if ok != tt.wantHit {
				t.Fatalf("fresh(%q, %v) hit = %v, want %v", tt.ask, tt.maxAge, ok, tt.wantHit)
			}
			if !ok {
				if got != nil {
					t.Errorf("fresh() missed but returned %v, want the zero value", got)
				}
				return
			}
			if got.InstanceID != "i-1" {
				t.Errorf("fresh() = %q, want the reading that was kept", got.InstanceID)
			}
		})
	}
}

// An empty memo must not answer, whatever maxAge says: with 0 meaning "never goes out of date" a zero timestamp would otherwise read as a valid reading and the pane would render an empty one forever.
func TestMetricsMemoWithNothingKeptNeverAnswers(t *testing.T) {
	memo := &metricsMemo[*InstanceMetrics]{}

	for _, maxAge := range []time.Duration{0, time.Minute} {
		if _, ok := memo.fresh("i-1", maxAge, time.Now()); ok {
			t.Errorf("an empty memo answered for maxAge %v", maxAge)
		}
	}
}

// The memo is what keeps a per-metric bill off a two-second redraw, so the fetch must run once per tier, not once per call.
func TestMemoizedFetchesOncePerTier(t *testing.T) {
	memo := &metricsMemo[*InstanceMetrics]{}
	fetches := 0
	fetch := func() (*InstanceMetrics, error) {
		fetches++

		return &InstanceMetrics{InstanceID: "i-1"}, nil
	}

	for range 5 {
		if _, err := memoized(memo, "i-1", time.Minute, fetch); err != nil {
			t.Fatalf("memoized() = %v", err)
		}
	}

	if fetches != 1 {
		t.Errorf("%d fetches for five redraws inside one tier, want 1", fetches)
	}

	// A different resource is a different reading and must cost its own call.
	if _, err := memoized(memo, "i-2", time.Minute, fetch); err != nil {
		t.Fatalf("memoized() for a second resource = %v", err)
	}
	if fetches != 2 {
		t.Errorf("%d fetches after selecting a second resource, want 2", fetches)
	}
}

// A throttled or denied GetMetricData must not be recorded as the answer: the pane keeps the last real reading and the next tick tries again, where caching the failure would blank the section until the selection moved.
func TestMemoizedDoesNotRecordAFailedFetch(t *testing.T) {
	memo := &metricsMemo[*InstanceMetrics]{}
	memo.keep("i-1", &InstanceMetrics{InstanceID: "i-1"}, time.Now().Add(-time.Hour))

	wantErr := errors.New("ThrottlingException: Rate exceeded")
	if _, err := memoized(memo, "i-1", time.Minute, func() (*InstanceMetrics, error) { return nil, wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("memoized() = %v, want the fetch's error", err)
	}

	got, ok := memo.fresh("i-1", 0, time.Now())
	if !ok {
		t.Fatal("the failed fetch evicted the previous reading, want it kept so the pane still has a number")
	}
	if got.InstanceID != "i-1" {
		t.Errorf("memo holds %v, want the reading from before the failure", got)
	}
}

// Each aggregate has to CONSULT its memo, which is what puts the metrics section on the slower tier; the wrappers themselves being correct proves nothing about the fetch reaching them.
// A nil SDK client is the one state a test can drive here: the fetch behind the memo can only fail, so a filled Metrics field with no metrics error means the answer came from the memo and nowhere else.
func TestEachOverviewAggregateAnswersItsMetricsSectionFromTheMemo(t *testing.T) {
	now := time.Now()

	t.Run("instance", func(t *testing.T) {
		client := &Client{}
		client.instanceMetrics.keep("i-1", &InstanceMetrics{InstanceID: "i-1"}, now)

		overview := client.GetInstanceOverview(context.Background(), "i-1", time.Minute)

		if err := overview.Err(SectionMetrics); err != nil {
			t.Fatalf("metrics section errored (%v), want it answered from the memo", err)
		}
		if overview.Metrics == nil || overview.Metrics.InstanceID != "i-1" {
			t.Errorf("Metrics = %v, want the memo's reading", overview.Metrics)
		}
	})

	t.Run("ecs cluster", func(t *testing.T) {
		client := &Client{}
		client.clusterMetrics.keep("app-cluster", &ECSClusterMetrics{ClusterName: "app-cluster"}, now)

		overview := client.GetECSClusterOverview(context.Background(), &ECSCluster{Name: "app-cluster", ContainerInsights: "enabled"}, time.Minute)

		if err := overview.Err(SectionMetrics); err != nil {
			t.Fatalf("metrics section errored (%v), want it answered from the memo", err)
		}
		if overview.Metrics == nil || overview.Metrics.ClusterName != "app-cluster" {
			t.Errorf("Metrics = %v, want the memo's reading", overview.Metrics)
		}
	})

	t.Run("ecs service", func(t *testing.T) {
		client := &Client{}
		client.serviceMetrics.keep("app-cluster/app-auth", &ECSServiceMetrics{ServiceName: "app-auth"}, now)

		overview := client.GetECSServiceOverview(context.Background(), &ECSService{Cluster: "app-cluster", Name: "app-auth"}, time.Minute)

		if err := overview.Err(SectionMetrics); err != nil {
			t.Fatalf("metrics section errored (%v), want it answered from the memo", err)
		}
		if overview.Metrics == nil || overview.Metrics.ServiceName != "app-auth" {
			t.Errorf("Metrics = %v, want the memo's reading", overview.Metrics)
		}
	})
}
