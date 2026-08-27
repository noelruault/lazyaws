package ui

import (
	"sync/atomic"
	"time"
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
