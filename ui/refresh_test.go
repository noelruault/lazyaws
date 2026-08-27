package ui

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/config"
)

var errFakeReload = errors.New("reload failed")

// readyTestClient satisfies Client.Ready, which is the gate the refresh tiers check before spending a call; the STS client is never invoked.
func readyTestClient() *aws.Client {
	return &aws.Client{STS: &sts.Client{}}
}

func TestTickIntervalTreatsNonPositiveSecondsAsOff(t *testing.T) {
	tests := []struct {
		seconds int
		want    time.Duration
	}{
		{seconds: -1, want: 0},
		{seconds: 0, want: 0},
		{seconds: 2, want: 2 * time.Second},
		{seconds: 60, want: time.Minute},
	}

	for _, tt := range tests {
		if got := tickInterval(tt.seconds); got != tt.want {
			t.Errorf("tickInterval(%d) = %v, want %v", tt.seconds, got, tt.want)
		}
	}
}

// A tick arriving while the previous reload is still running must be DROPPED, not queued: the reload in flight is already fetching the state the tick wanted.
func TestSingleFlightDropsACallThatFindsTheReloaderRunning(t *testing.T) {
	var calls atomic.Int32
	entered, release := make(chan struct{}, 1), make(chan struct{})

	reload := singleFlight(func() error {
		calls.Add(1)
		entered <- struct{}{}
		<-release

		return nil
	})

	firstReturned := make(chan struct{})
	go func() {
		_ = reload()
		close(firstReturned)
	}()
	<-entered

	// The dropped call returns rather than blocking, which is what keeps a tick from stacking up behind a slow account.
	done := make(chan struct{})
	go func() {
		_ = reload()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("a call made while the reloader was running blocked instead of being dropped")
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("reloader ran %d times while one call was in flight, want 1: the second call was queued instead of dropped", got)
	}

	// Waiting for the first call to RETURN, not just for its release: the guard clears in a defer, so a call made the instant release lands would be dropped for a reason the test is not about.
	close(release)
	<-firstReturned

	// The guard has to clear, or one slow reload would silence the panel for the rest of the session.
	if err := reload(); err != nil {
		t.Fatalf("reload() after the first returned = %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("reloader ran %d times in total, want 2: the in-flight guard never cleared", got)
	}
}

// The wrapper must not swallow what the loader reports, or a panel whose fetch is failing looks like a panel that reloaded fine.
func TestSingleFlightReturnsTheReloadersError(t *testing.T) {
	want := errFakeReload
	if got := singleFlight(func() error { return want })(); got != want {
		t.Errorf("singleFlight(...)() = %v, want %v", got, want)
	}
}

// goEvery is what puts a tier on a clock, and a subprocess owning the tty must stop every tier: a reload writing to the view while a child process holds the terminal corrupts both.
func TestGoEveryRunsOnlyWhileBackgroundThreadsAreNotPaused(t *testing.T) {
	gui := &Gui{}
	gui.PauseBackgroundThreads.Store(true)

	var calls atomic.Int32
	gui.goEvery(time.Millisecond, func() error {
		calls.Add(1)

		return nil
	})

	// goEvery fires once up front by contract (a ticker does not tick immediately), so the paused count is exactly that one call.
	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("with background threads paused the function ran %d times, want only the up-front call", got)
	}

	gui.PauseBackgroundThreads.Store(false)

	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("after unpausing the function ran %d times, want it ticking again", calls.Load())
		}
		time.Sleep(time.Millisecond)
	}
}

// The panel tier reloads exactly one list per tick, and it is the list on screen: seven of the eight panels would be describing rows nobody is looking at.
// Headless rather than newTestGui, because the tier resolves its panel through the focused view's NAME and views built as literals have none: the fallback would answer "profile" for every stack and the test would pass on the wrong panel.
func TestReloadFocusedPanelTriggersOnlyTheFocusedPanelsThrottle(t *testing.T) {
	gui, _ := newHeadlessGui(t)
	gui.Client = readyTestClient()

	var triggered sync.Map
	gui.panelThrottles = map[string]*throttle{}
	for name := range gui.panelReloaders() {
		gui.panelThrottles[name] = newThrottle(time.Hour, func() { triggered.Store(name, true) })
	}

	// main on top of ec2 is focus having moved into the detail pane, which must keep refreshing the list that pane belongs to.
	gui.State.ViewStack = []string{"ec2", "main"}

	if err := gui.reloadFocusedPanel(); err != nil {
		t.Fatalf("reloadFocusedPanel() = %v", err)
	}

	for name := range gui.panelReloaders() {
		_, fired := triggered.Load(name)
		if want := name == "ec2"; fired != want {
			t.Errorf("panel %q triggered = %v, want %v: the tier reloads the list on screen and nothing else", name, fired, want)
		}
	}
}

// With no credentials every panel but the profile list can only fail, and the profile list already reloads through refresh; a tier spending eight failing calls every couple of seconds is how a bad profile becomes a busy loop.
func TestReloadFocusedPanelIsInertWithoutCredentials(t *testing.T) {
	gui := newTestGui(t)
	gui.State.ViewStack = []string{"ec2"}

	var triggered atomic.Bool
	gui.panelThrottles = map[string]*throttle{
		"ec2": newThrottle(time.Hour, func() { triggered.Store(true) }),
	}

	if err := gui.reloadFocusedPanel(); err != nil {
		t.Fatalf("reloadFocusedPanel() with no client = %v", err)
	}
	if triggered.Load() {
		t.Error("the tier reloaded a panel with no AWS client, want it to wait for credentials")
	}

	gui.Client = readyTestClient()
	gui.authProblem = errFakeReload

	if err := gui.reloadFocusedPanel(); err != nil {
		t.Fatalf("reloadFocusedPanel() with an auth problem = %v", err)
	}
	if triggered.Load() {
		t.Error("the tier reloaded a panel while an auth problem was showing, want it to wait for a working profile")
	}
}

// The tier looks its panel up by the focused VIEW's name, so the two name sets are one contract: rename either side and that panel silently stops auto-refreshing, with nothing else broken to notice.
func TestEveryPanelReloaderIsNamedAfterItsSideView(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	reloaders := gui.panelReloaders()
	for _, name := range sidePanelViewNames(gui.allSidePanels()) {
		if _, ok := reloaders[name]; !ok {
			t.Errorf("side view %q has no reloader of that name, so the panel tier cannot reload it", name)
		}
	}

	views := map[string]bool{}
	for _, name := range sidePanelViewNames(gui.allSidePanels()) {
		views[name] = true
	}
	for name := range reloaders {
		if !views[name] {
			t.Errorf("reloader %q matches no side view name, so no tick will ever reach it", name)
		}
	}
}

// A tick, a manual r and a throttle firing are three callers of one loader; the guard is built once in NewGui so none of them can reload a list that is already reloading.
func TestNewGuiPutsEveryPanelLoaderBehindOneSingleFlightGuard(t *testing.T) {
	gui, err := NewGui(&config.Config{User: config.DefaultUserConfig()}, nil, make(chan error, 1))
	if err != nil {
		t.Fatalf("NewGui() = %v", err)
	}

	for name := range gui.panelReloaders() {
		if _, ok := gui.panelReloads[name]; !ok {
			t.Errorf("panel %q has no single-flighted loader, so two callers can reload it at once", name)
		}
	}
	if len(gui.panelReloads) != len(gui.panelThrottles) {
		t.Errorf("%d single-flighted loaders against %d throttles, want one each", len(gui.panelReloads), len(gui.panelThrottles))
	}
}
