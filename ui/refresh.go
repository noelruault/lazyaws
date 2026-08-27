package ui

import (
	"sync"
	"sync/atomic"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
)

// profileReloader names the one loader the auth-problem path reaches for by name, so that string cannot drift from the key panelReloaders registers it under.
const profileReloader = "profile"

// tickInterval maps configured seconds onto a refresh interval, where 0 means no auto-refresh at all.
// time.NewTicker panics on a non-positive duration, so a negative value is folded into the off state rather than passed on; config.RefreshInterval cannot be used for these tiers because it substitutes a fallback for every non-positive value, which would make 0 mean "the default" instead of "off".
func tickInterval(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}

	return time.Duration(seconds) * time.Second
}

// metricsMaxAge is how long a CloudWatch reading stays the answer for the pane holding it.
// Named rather than read inline at each overview, so the three panes that read metrics cannot end up on three different tiers: this is the interval with a price attached, since GetMetricData is billed per metric requested.
func (gui *Gui) metricsMaxAge() time.Duration {
	return gui.Config.User.Refresh.MetricsInterval()
}

// singleFlight wraps a panel reloader so a call that finds the previous one still running is DROPPED rather than queued.
// Queueing is the wrong answer for a refresh: the caller wanted the current state and the reload already in flight is fetching exactly that, so a queue only converts a slow account into a backlog of identical list fetches that all land at once and each cost a full set of AWS calls.
func singleFlight(reload func() error) func() error {
	var running atomic.Bool

	return func() error {
		if !running.CompareAndSwap(false, true) {
			return nil
		}
		defer running.Store(false)

		return reload()
	}
}

// backoffCeiling caps how far a throttled pane's refresh interval is stretched.
// 60s is the metrics tier's own interval: beyond it a pane that is merely being rate-limited would refresh more slowly than the slowest tier this app has, which reads as a hung pane rather than as a pane giving AWS room.
const backoffCeiling = 60 * time.Second

// isThrottle reports whether AWS answered a fetch by asking for a slower send rate.
// The SDK's own throttle set is used rather than a hand-written list: it is the same classification the adaptive retryer acts on, it covers the codes the services here actually emit (SlowDown from S3, EC2ThrottledException, RequestLimitExceeded, ThrottlingException), and a list written out here would silently fall behind the SDK's on the next service that invents its own spelling.
func isThrottle(err error) bool {
	if err == nil {
		return false
	}

	return retry.ThrottleErrorCode{Codes: retry.DefaultThrottleErrorCodes}.IsErrorThrottle(err) == awssdk.TrueTernary
}

// throttleWatch carries the verdict on the last overview fetch from the render that made it to the gate that paces the next one.
// One watch serves every pane because the task manager runs exactly one main-panel task at a time, so the fetch that last reported IS the open pane's; a verdict left behind by the pane the user just navigated away from is still a true statement about the account being throttled, and the incoming pane's first fetch is never delayed by it.
type throttleWatch struct {
	mu        sync.Mutex
	throttled bool
	reported  bool
}

// observe records whether any of a fetch's errors was AWS asking for a slower rate.
func (w *throttleWatch) observe(errs ...error) {
	throttled := false
	for _, err := range errs {
		if isThrottle(err) {
			throttled = true

			break
		}
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.throttled, w.reported = throttled, true
}

// observeSections records the verdict for an overview's per-section error map.
func (w *throttleWatch) observeSections(errs map[string]error) {
	for _, err := range errs {
		if isThrottle(err) {
			w.observe(err)

			return
		}
	}

	w.observe()
}

// take reports the verdict on the fetch since the last call, and whether there was one at all.
// reported is what stops a pane from decaying its own backoff on ticks it never fetched on: without it a 60s backoff would unwind over a handful of dropped ticks, having learnt nothing about whether AWS is still throttling.
func (w *throttleWatch) take() (throttled, reported bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	throttled, reported = w.throttled, w.reported
	w.throttled, w.reported = false, false

	return throttled, reported
}

// nextInterval widens a throttled pane's refresh interval and narrows a clean one back towards its configured base.
// Doubling then halving is the standard exponential shape: the pane gives ground at once while AWS is pushing back, and comes back to its configured rate over several clean fetches rather than in one, so a single lucky response does not immediately restore the rate that earned the throttle.
func nextInterval(current, base time.Duration, throttled bool) time.Duration {
	if throttled {
		return min(max(current, base)*2, backoffCeiling)
	}

	return max(current/2, base)
}

// paneGate widens the gap between a ticking overview's fetches while AWS is throttling them, by dropping ticks: a ticker task cannot be re-armed with a new duration once built, so this is the ticker's EFFECTIVE interval.
// One gate per task, which is what makes a new selection, resize or profile switch start clean, and why it needs no lock: NewTickerTask calls its function sequentially and nothing else reaches the gate.
type paneGate struct {
	base     time.Duration
	interval time.Duration
	last     time.Time
	watch    *throttleWatch
}

func newPaneGate(base time.Duration, watch *throttleWatch) *paneGate {
	return &paneGate{base: base, interval: base, watch: watch}
}

// due reports whether this tick may fetch, after folding in what the last fetch learnt about being throttled.
func (g *paneGate) due(now time.Time) bool {
	if throttled, reported := g.watch.take(); reported {
		g.interval = nextInterval(g.interval, g.base, throttled)
	}

	if !g.last.IsZero() && now.Sub(g.last) < g.interval {
		return false
	}

	g.last = now

	return true
}

// startAutoRefresh puts the side panel the user is looking at on its own refresh tier.
// Exactly ONE panel reloads per tick: the eight list fetches are individually cheap and collectively the app's largest recurring cost, and seven of them would be describing rows nobody is looking at.
func (gui *Gui) startAutoRefresh() {
	interval := tickInterval(gui.Config.User.Refresh.PanelSeconds)
	if interval <= 0 {
		return
	}

	gui.goEvery(interval, gui.reloadFocusedPanel)
}

// reloadFocusedPanel reloads the panel whose list is on screen, resolved through focus history rather than through the focused view so a tick keeps refreshing the list its open detail pane belongs to once focus has moved into main.
// It triggers the panel's throttle instead of calling the loader, so a tick landing next to a manual r collapses into one reload; the loader behind that throttle is single-flighted, which is what makes a tick arriving mid-reload free.
func (gui *Gui) reloadFocusedPanel() error {
	// The profile panel is the recovery path and reloads itself through refresh; with no credentials every other panel's tick is a call that can only fail.
	if gui.authProblem != nil || !gui.Client.Ready() {
		return nil
	}

	panelThrottle, ok := gui.panelThrottles[gui.currentSideViewName()]
	if !ok {
		return nil
	}

	panelThrottle.Trigger()

	return nil
}
