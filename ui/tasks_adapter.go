package ui

import (
	"context"
	"time"

	"github.com/jesseduffield/gocui"
	"github.com/noelruault/lazyaws/ui/tasks"
)

func (gui *Gui) QueueTask(f func(ctx context.Context)) error {
	return gui.taskManager.NewTask(f)
}

type RenderStringTaskOpts struct {
	Autoscroll    bool
	Wrap          bool
	GetStrContent func() string
}

type TaskOpts struct {
	Autoscroll bool
	Wrap       bool
	Func       func(ctx context.Context)
}

func (gui *Gui) NewRenderStringTask(opts RenderStringTaskOpts) tasks.TaskFunc {
	taskOpts := TaskOpts{
		Autoscroll: opts.Autoscroll,
		Wrap:       opts.Wrap,
		Func: func(ctx context.Context) {
			gui.RenderStringMain(opts.GetStrContent())
		},
	}

	return gui.NewTask(taskOpts)
}

// NewSimpleRenderStringTask assumes it's cheap to obtain the content (otherwise pass a function that returns the content).
func (gui *Gui) NewSimpleRenderStringTask(getContent func() string) tasks.TaskFunc {
	return gui.NewRenderStringTask(RenderStringTaskOpts{
		GetStrContent: getContent,
		Autoscroll:    false,
		Wrap:          gui.Config.User.Gui.WrapMainPanel,
	})
}

func (gui *Gui) NewTask(opts TaskOpts) tasks.TaskFunc {
	return func(ctx context.Context) {
		// TaskManager runs off the UI thread, so view mutations must be queued through gocui.
		gui.g.Update(func(*gocui.Gui) error {
			mainView := gui.Views.Main
			mainView.Autoscroll = opts.Autoscroll
			mainView.Wrap = opts.Wrap
			return nil
		})

		opts.Func(ctx)
	}
}

type TickerTaskOpts struct {
	Duration   time.Duration
	Before     func(ctx context.Context)
	Func       func(ctx context.Context, notifyStopped chan struct{})
	Autoscroll bool
	Wrap       bool
}

// NewTickerTask keeps its ticker inline to avoid nesting TaskManager ownership.
func (gui *Gui) NewTickerTask(opts TickerTaskOpts) tasks.TaskFunc {
	notifyStopped := make(chan struct{}, 10)

	task := func(ctx context.Context) {
		if opts.Before != nil {
			opts.Before(ctx)
		}
		tickChan := time.NewTicker(opts.Duration)
		defer tickChan.Stop()
		opts.Func(ctx, notifyStopped)
		for {
			select {
			case <-notifyStopped:
				return
			case <-ctx.Done():
				return
			case <-tickChan.C:
				opts.Func(ctx, notifyStopped)
			}
		}
	}

	return gui.NewTask(TaskOpts{
		Autoscroll: opts.Autoscroll,
		Wrap:       opts.Wrap,
		Func:       task,
	})
}
