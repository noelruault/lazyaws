package aws

import (
	"context"
	"sync"
	"time"
)

// metricsMemo holds the last CloudWatch reading for ONE resource, so a pane that redraws every couple of seconds does not re-pay for metrics CloudWatch publishes once a minute.
// This cache exists because GetMetricData is billed per metric requested and the panes it serves ask for six to eight metrics per redraw, which is the one place in this app where a refresh interval has a price attached rather than a latency cost.
// One entry rather than a map: every pane that reads metrics shows a single selection at a time, so a second entry could only ever be the row the user just left. A profile switch replaces the whole Client (ui/profile_panel.go), which is what keeps a reading from one account from ever answering for another.
type metricsMemo[T any] struct {
	mu    sync.Mutex
	key   string
	at    time.Time
	value T
}

// fresh reports the reading held for key when it is still current, where maxAge of 0 means it never goes out of date.
// That is the meaning config.RefreshConfig.MetricsInterval gives 0 — metrics auto-refresh switched off, so the reading taken for this selection is the answer until the selection moves — and the two must agree or switching the tier off would make every redraw refetch instead of none.
func (m *metricsMemo[T]) fresh(key string, maxAge time.Duration, now time.Time) (T, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.key != key || m.at.IsZero() {
		var zero T

		return zero, false
	}

	if maxAge > 0 && now.Sub(m.at) >= maxAge {
		var zero T

		return zero, false
	}

	return m.value, true
}

// keep records a reading as the answer for key from now on.
func (m *metricsMemo[T]) keep(key string, value T, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.key, m.at, m.value = key, now, value
}

// GetInstanceMetricsAged answers from the instance metrics memo while its reading is still current, and otherwise fetches one.
// Every pane showing EC2 metrics goes through here rather than through GetInstanceMetrics: instanceMetricQueries asks for a 300-second period because that is what basic monitoring publishes, so a pane redrawing on its own faster interval re-pays a per-metric bill for a number that cannot have changed. maxAge of 0 means the reading stays the answer for as long as the instance is selected.
func (c *Client) GetInstanceMetricsAged(ctx context.Context, instanceID string, maxAge time.Duration) (*InstanceMetrics, error) {
	return memoized(&c.instanceMetrics, instanceID, maxAge, func() (*InstanceMetrics, error) {
		return c.GetInstanceMetrics(ctx, instanceID)
	})
}

// memoized answers from the memo while its reading for key is current, and otherwise fetches one and records it.
// A failed fetch is NOT recorded: the previous reading stays, so one throttled GetMetricData costs the pane a stale number rather than an empty section, and the next tick tries again.
func memoized[T any](memo *metricsMemo[T], key string, maxAge time.Duration, fetch func() (T, error)) (T, error) {
	if value, ok := memo.fresh(key, maxAge, time.Now()); ok {
		return value, nil
	}

	value, err := fetch()
	if err != nil {
		return value, err
	}

	memo.keep(key, value, time.Now())

	return value, nil
}
