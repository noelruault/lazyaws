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

// overviewTab is the Overview tab a resource panel opens on, refreshing on the configured interval.
func overviewTab[T any](gui *Gui, render func(ctx context.Context, item T, width int) string) panels.MainTab[T] {
	return overviewTabEvery(gui, func() time.Duration {
		return tickInterval(gui.Config.User.Refresh.OverviewSeconds)
	}, render)
}

// staticOverviewTab is the Overview tab for a resource whose overview is configuration rather than state: it renders once per selection.
// A bucket costs eleven S3 calls and a repository pages its whole image list, so a two-second ticker would spend unbounded calls redrawing an unchanged pane; selection, profile switch and resize each still rebuild the task.
func staticOverviewTab[T any](gui *Gui, render func(ctx context.Context, item T, width int) string) panels.MainTab[T] {
	return overviewTabEvery(gui, func() time.Duration { return 0 }, render)
}

// overviewTabEvery builds the Overview tab both variants share, so neither can drift from the other's key or title.
// render is handed main's inner width because an overview sizes its own columns and wrapping is off; the width is read HERE rather than inside the task, because Render runs on the UI loop and the task does not, and a gocui view read off the loop races the render.
// interval is a function for the same reason: it is resolved per render, so a session that changes the refresh setting does not keep the interval its first render was built with.
func overviewTabEvery[T any](gui *Gui, interval func() time.Duration, render func(ctx context.Context, item T, width int) string) panels.MainTab[T] {
	return panels.MainTab[T]{
		Key:   overviewTabKey,
		Title: "Overview",
		Render: func(item T) tasks.TaskFunc {
			width := gui.Views.Main.InnerWidth()

			return gui.newOverviewTask(interval(), func(ctx context.Context) string {
				return render(ctx, item, width)
			})
		},
	}
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

// overviewUnavailableBecause keeps a failed fetch on the same footing as an absent one, and says which it was.
// A ticking overview retries on its own interval, so the reason has to stay on screen: without it a transient throttle and a permanent denial look identical.
func overviewUnavailableBecause(kind string, err error) string {
	return overviewUnavailable(kind) + "\n" + utils.ColoredString(err.Error(), color.FgRed)
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
