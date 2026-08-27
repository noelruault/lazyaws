package ui

import (
	"context"
	"time"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/tasks"
	"github.com/noelruault/lazyaws/ui/utils"
)

// overviewTabKey is the tab's half of ContextState.GetCurrentContextKey, so it also decides when an open overview counts as unchanged.
const overviewTabKey = "overview"

// overviewTab is the Overview tab a resource panel opens on.
// render is handed main's inner width because an overview sizes its own columns and wrapping is off; the width is read HERE rather than inside the task, because Render runs on the UI loop and the task does not, and a gocui view read off the loop races the render.
func overviewTab[T any](gui *Gui, render func(ctx context.Context, item T, width int) string) panels.MainTab[T] {
	return panels.MainTab[T]{
		Key:   overviewTabKey,
		Title: "Overview",
		Render: func(item T) tasks.TaskFunc {
			width := gui.Views.Main.InnerWidth()

			return gui.newOverviewTask(overviewInterval(gui.Config.User.Refresh.OverviewSeconds), func(ctx context.Context) string {
				return render(ctx, item, width)
			})
		},
	}
}

// overviewInterval maps the configured seconds onto a duration, where 0 means no auto-refresh at all.
// time.NewTicker panics on a non-positive duration, so a negative value is folded into the off state rather than passed on.
func overviewInterval(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}

	return time.Duration(seconds) * time.Second
}

// newOverviewTask re-renders every interval, or exactly once when interval is 0.
// Wrap stays off on both paths: the content is already laid out for a known width, and a soft wrap would fold the second column under the first.
func (gui *Gui) newOverviewTask(interval time.Duration, render func(context.Context) string) tasks.TaskFunc {
	if interval <= 0 {
		return gui.NewTask(TaskOpts{
			Wrap: false,
			Func: func(ctx context.Context) { gui.renderOverview(ctx, render) },
		})
	}

	return gui.NewTickerTask(TickerTaskOpts{
		Duration: interval,
		Wrap:     false,
		Before:   func(context.Context) { gui.clearMainView() },
		Func:     func(ctx context.Context, _ chan struct{}) { gui.renderOverview(ctx, render) },
	})
}

// renderOverview drops a render whose data was fetched under a profile that has since been switched away from.
// The generation is snapshotted before render runs, not after, or a switch that lands mid-fetch would compare the new generation against itself.
func (gui *Gui) renderOverview(ctx context.Context, render func(context.Context) string) {
	gen := gui.Gen

	content := render(ctx)
	if gen != gui.Gen {
		return
	}

	gui.reRenderStringMain(content)
}

// overviewUnavailable names the resource so an overview with nothing to show is a statement rather than a blank pane.
func overviewUnavailable(kind string) string {
	return utils.ColoredString(kind+" overview unavailable", color.Faint)
}

// syncMainWidth re-renders the open tab when main's inner width changes.
// It runs from layout for the same reason syncQWidth does: that is the one place view dimensions can be read without racing the render.
// An overview is laid out for the width captured when its task was built and wrapping is off, so after a resize the old text is either cut off or leaves its second column stranded.
func (gui *Gui) syncMainWidth() {
	width := gui.Views.Main.InnerWidth()
	if width == gui.mainWidth {
		return
	}

	gui.mainWidth = width
	gui.rerenderMainTab.Trigger()
}

// rerenderCurrentMainTab rebuilds the open tab's task at the new width.
// Clearing ObjectKey is what lets it through: the selection has not changed, so ShouldRefresh would otherwise reuse the task that was already laid out for the old width.
func (gui *Gui) rerenderCurrentMainTab() {
	gui.g.Update(func(*gocui.Gui) error {
		// The chat screen owns main while it is up, and it rewraps itself from syncQWidth.
		if gui.mainBelongsToQ() {
			return nil
		}

		panel, ok := gui.sidePanelForMain()
		if !ok {
			return nil
		}

		gui.State.Panels.Main.ObjectKey = ""

		return panel.HandleSelect()
	})
}
