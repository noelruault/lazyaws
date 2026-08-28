package ui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/ui/tasks"
	"github.com/noelruault/lazyaws/ui/utils"
)

func TestOverviewInterval(t *testing.T) {
	tests := []struct {
		name    string
		seconds int
		want    time.Duration
	}{
		{name: "the default", seconds: 2, want: 2 * time.Second},
		{name: "a slow refresh", seconds: 60, want: 60 * time.Second},
		{name: "the fastest a config can ask for", seconds: 1, want: time.Second},
		{name: "zero turns it off", seconds: 0, want: 0},
		{name: "a negative cannot reach time.NewTicker", seconds: -5, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tickInterval(tt.seconds); got != tt.want {
				t.Errorf("tickInterval(%d) = %v, want %v", tt.seconds, got, tt.want)
			}
		})
	}
}

// An overview that fails to fetch re-renders on its own interval, so it has to say WHY: without the reason a transient throttle and a permanent access denial are the same blank statement.
func TestOverviewUnavailableCarriesTheReason(t *testing.T) {
	got := utils.Decolorise(overviewUnavailableBecause("secret", errors.New("AccessDeniedException: not authorized")))

	if want := "secret overview unavailable"; !strings.Contains(got, want) {
		t.Errorf("overview = %q, want it to contain %q", got, want)
	}
	if want := "AccessDeniedException: not authorized"; !strings.Contains(got, want) {
		t.Errorf("overview = %q, want it to carry the reason %q", got, want)
	}
}

// counter records how often an overview asked for its content, which is the only visible difference between the ticking and the one-shot task paths.
type counter struct {
	mu sync.Mutex
	n  int
}

func (c *counter) render(context.Context) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++

	return "overview body"
}

func (c *counter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.n
}

// atLeast polls because the task runs on its own goroutine; it reports the count it settled on.
func (c *counter) atLeast(want int, within time.Duration) int {
	for deadline := time.Now().Add(within); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
		if c.count() >= want {
			break
		}
	}

	return c.count()
}

func TestOverviewTaskWithoutAnIntervalRendersOnce(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	var calls counter
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := gui.newOverviewTask(0, "t-once", calls.render)
	go task(ctx)

	if got := calls.atLeast(1, time.Second); got != 1 {
		t.Fatalf("renders = %d, want 1 before the wait", got)
	}

	// Long enough that the 15ms ticker the sibling test uses would have fired several times.
	time.Sleep(150 * time.Millisecond)
	if got := calls.count(); got != 1 {
		t.Errorf("renders = %d after waiting, want 1: an interval of 0 must not start a ticker", got)
	}
}

func TestOverviewTaskRepeatsOnItsInterval(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	var calls counter
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := gui.newOverviewTask(15*time.Millisecond, "t-tick", calls.render)
	go task(ctx)

	if got := calls.atLeast(3, time.Second); got < 3 {
		t.Errorf("renders = %d, want at least 3: a non-zero interval must keep re-rendering", got)
	}
}

// An overview is laid out for a known width, so main must have wrapping off however the task was built.
// Main arrives here wrapped, because Gui.WrapMainPanel defaults to true and resetMainView puts it back on every time focus moves between side panels.
func TestOverviewTurnsWrappingOff(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
	}{
		{name: "one-shot", interval: 0},
		{name: "ticking", interval: 15 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gui, g := newHeadlessGui(t)
			run(t, g, func() error { gui.Views.Main.Wrap = true; return nil })

			var calls counter
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go gui.newOverviewTask(tt.interval, "t-wrap", calls.render)(ctx)
			calls.atLeast(1, time.Second)

			wrapped := true
			for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
				wrapped = ask(g, func() bool { return gui.Views.Main.Wrap })
				if !wrapped {
					break
				}
			}
			if wrapped {
				t.Error("main is still wrapping, want wrap off so the overview's columns survive")
			}
		})
	}
}

// A profile switch invalidates whatever the previous profile's credentials were fetching, so a render that lands afterwards must not reach the screen.
func TestOverviewDropsAResultFromASupersededProfile(t *testing.T) {
	gui, g := newHeadlessGui(t)
	resizeView(t, g, "main", 60, 10)

	ctx := context.Background()

	gui.Gen = 1
	gui.renderOverview(ctx, "t-gen", func(context.Context) string { return "current profile" })
	if got := mainBufferWithin(g, gui, "current profile", time.Second); !strings.Contains(got, "current profile") {
		t.Fatalf("main = %q, want the render made under the live profile", got)
	}

	gui.renderOverview(ctx, "t-gen", func(context.Context) string {
		gui.Gen++

		return "stale profile"
	})

	// Polled rather than read once, because reRenderStringMain lands through a queued gocui update.
	for deadline := time.Now().Add(150 * time.Millisecond); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if got := ask(g, func() string { return gui.Views.Main.Buffer() }); strings.Contains(got, "stale profile") {
			t.Fatalf("main = %q, want the superseded render dropped", got)
		}
	}
}

// The overview sizes its own columns, so the width it is built with has to be main's real inner width and not a guess.
func TestOverviewTabCapturesMainsInnerWidth(t *testing.T) {
	gui, g := newHeadlessGui(t)
	resizeView(t, g, "main", 64, 12)

	widths := make(chan int, 4)
	tab := overviewTab(gui, func(item string) string { return "t-" + item }, func(_ context.Context, item string, width int) string {
		widths <- width

		return item
	})

	if tab.Key != overviewTabKey || tab.Title != "Overview" {
		t.Fatalf("tab = {%q %q}, want {%q %q}", tab.Key, tab.Title, overviewTabKey, "Overview")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Render belongs on the UI loop: that is the contract the width capture relies on.
	task := ask(g, func() tasks.TaskFunc { return tab.Render("body") })
	go task(ctx)

	want := ask(g, func() int { return gui.Views.Main.InnerWidth() })
	select {
	case got := <-widths:
		if got != want {
			t.Errorf("render was given width %d, want main's inner width %d", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("the overview task never rendered")
	}
}

// Layout runs on every event, so a re-render must cost something only when the width actually moved.
func TestSyncMainWidthRerendersOnlyWhenTheWidthMoves(t *testing.T) {
	gui, g := newHeadlessGui(t)

	var calls counter
	gui.rerenderMainTab = newThrottle(time.Millisecond, func() { calls.render(context.Background()) })

	resizeView(t, g, "main", 60, 10)
	run(t, g, func() error { gui.syncMainWidth(); return nil })
	if got := calls.count(); got != 1 {
		t.Fatalf("re-renders = %d after the first width, want 1", got)
	}

	time.Sleep(5 * time.Millisecond)
	run(t, g, func() error { gui.syncMainWidth(); return nil })
	if got := calls.count(); got != 1 {
		t.Errorf("re-renders = %d after an unchanged width, want 1", got)
	}

	resizeView(t, g, "main", 90, 10)
	time.Sleep(5 * time.Millisecond)
	run(t, g, func() error { gui.syncMainWidth(); return nil })
	if got := calls.count(); got != 2 {
		t.Errorf("re-renders = %d after a resize, want 2", got)
	}
}

// rerenderCurrentMainTab has to clear ObjectKey, or ShouldRefresh keeps the task that was laid out for the old width.
func TestRerenderCurrentMainTabClearsTheObjectKey(t *testing.T) {
	gui, g := newHeadlessGui(t)

	gui.State.ViewStack = []string{"ec2", "main"}
	gui.State.Panels.Main.ObjectKey = "ec2-i-123-overview"

	gui.rerenderCurrentMainTab()

	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
		if ask(g, func() string { return gui.State.Panels.Main.ObjectKey }) != "ec2-i-123-overview" {
			return
		}
	}

	t.Error("ObjectKey still holds the pre-resize key, so ShouldRefresh will skip the re-render")
}

// The chat screen owns main while it is up and rewraps itself from syncQWidth, so a resize must not make a side panel redraw over the transcript.
func TestRerenderCurrentMainTabLeavesTheChatScreenAlone(t *testing.T) {
	fakeQOnPath(t, "exit 0")
	gui, g := newHeadlessGui(t)

	run(t, g, gui.handleToggleQ)
	if !ask(g, gui.qScreenActive) {
		t.Fatal("the chat screen is not up, so this test would pass for the wrong reason")
	}

	gui.State.ViewStack = append(gui.State.ViewStack, "ec2")
	gui.State.Panels.Main.ObjectKey = "chat"

	gui.rerenderCurrentMainTab()

	for deadline := time.Now().Add(150 * time.Millisecond); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if got := ask(g, func() string { return gui.State.Panels.Main.ObjectKey }); got != "chat" {
			t.Fatalf("ObjectKey = %q, want the chat screen left holding main", got)
		}
	}
}

// Re-selecting a resource must paint its last pane instantly, and a profile switch must drop every pane the previous account rendered.
func TestOverviewPaneCache(t *testing.T) {
	var cache overviewPaneCache

	if _, ok := cache.get(1, "ec2-i-1-w80"); ok {
		t.Fatal("an empty cache answered")
	}

	cache.put(1, "ec2-i-1-w80", "pane")
	if got, ok := cache.get(1, "ec2-i-1-w80"); !ok || got != "pane" {
		t.Fatalf("get = %q, %v after put", got, ok)
	}
	// A pane laid out for another width is a miss, not a misfit.
	if _, ok := cache.get(1, "ec2-i-1-w120"); ok {
		t.Error("a different width hit the cache")
	}
	if _, ok := cache.get(2, "ec2-i-1-w80"); ok {
		t.Error("a later generation read the previous account's pane")
	}

	cache.put(2, "ec2-i-1-w80", "new account")
	if _, ok := cache.get(1, "ec2-i-1-w80"); ok {
		t.Error("the previous generation's pane survived the switch")
	}
}

// The opening paint is what replaces the blank pane: the cached render when the resource was seen before, a loading line when it was not.
func TestPaintOverviewOpening(t *testing.T) {
	gui, g := newHeadlessGui(t)
	resizeView(t, g, "main", 60, 10)

	gui.paintOverviewOpening("first-visit")
	if got := mainBufferWithin(g, gui, "loading overview…", time.Second); !strings.Contains(got, "loading overview…") {
		t.Fatalf("main = %q, want the loading line on a first visit", got)
	}

	gui.overviewCache.put(gui.Gen, "revisit", "the pane from last time")
	gui.paintOverviewOpening("revisit")
	if got := mainBufferWithin(g, gui, "the pane from last time", time.Second); !strings.Contains(got, "the pane from last time") {
		t.Fatalf("main = %q, want the cached pane on a revisit", got)
	}
}

func mainBufferWithin(g *gocui.Gui, gui *Gui, want string, within time.Duration) string {
	var got string
	for deadline := time.Now().Add(within); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
		got = ask(g, func() string { return gui.Views.Main.Buffer() })
		if strings.Contains(got, want) {
			break
		}
	}

	return got
}
