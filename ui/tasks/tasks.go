// Package tasks serializes cancellable main-panel work so navigation can stop stale AWS fetches.
package tasks

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type TaskManager struct {
	currentTask  *Task
	waitingMutex sync.Mutex
	taskIDMutex  sync.Mutex
	Log          *slog.Logger
	newTaskId    int
}

type Task struct {
	ctx           context.Context
	cancel        context.CancelFunc
	stopped       bool
	stopMutex     sync.Mutex
	notifyStopped chan struct{}
	Log           *slog.Logger
	f             func(ctx context.Context)
}

type TaskFunc func(ctx context.Context)

func NewTaskManager(log *slog.Logger) *TaskManager {
	return &TaskManager{Log: log}
}

func (t *TaskManager) Close() {
	if t.currentTask == nil {
		return
	}

	c := make(chan struct{}, 1)

	go func() {
		t.currentTask.Stop()
		c <- struct{}{}
	}()

	select {
	case <-c:
		return
	case <-time.After(3 * time.Second):
		// Printing here would land on the terminal gocui is drawing to and scroll every later frame off by a row.
		slog.Warn("could not kill child process within the grace period")
	}
}

func (t *TaskManager) NewTask(f func(ctx context.Context)) error {
	go func() {
		t.taskIDMutex.Lock()
		t.newTaskId++
		taskID := t.newTaskId
		t.taskIDMutex.Unlock()

		t.waitingMutex.Lock()
		defer t.waitingMutex.Unlock()
		t.taskIDMutex.Lock()
		if taskID < t.newTaskId {
			t.taskIDMutex.Unlock()
			return
		}
		t.taskIDMutex.Unlock()

		ctx, cancel := context.WithCancel(context.Background())
		notifyStopped := make(chan struct{})

		if t.currentTask != nil {
			t.Log.Info("asking task to stop")
			t.currentTask.Stop()
			t.Log.Info("task stopped")
		}

		t.currentTask = &Task{
			ctx:           ctx,
			cancel:        cancel,
			notifyStopped: notifyStopped,
			Log:           t.Log,
			f:             f,
		}

		go func() {
			f(ctx)
			t.Log.Info("returned from function, closing notifyStopped")
			close(notifyStopped)
		}()
	}()

	return nil
}

func (t *Task) Stop() {
	t.stopMutex.Lock()
	defer t.stopMutex.Unlock()
	if t.stopped {
		return
	}

	t.cancel()
	t.Log.Info("closed stop channel, waiting for notifyStopped message")
	<-t.notifyStopped
	t.Log.Info("received notifystopped message")
	t.stopped = true
}

// NewTickerTask acquires ownership before before; only cancellation or notifyStopped ends repetition.
func (t *TaskManager) NewTickerTask(duration time.Duration, before func(ctx context.Context), f func(ctx context.Context, notifyStopped chan struct{})) error {
	notifyStopped := make(chan struct{}, 10)

	return t.NewTask(func(ctx context.Context) {
		if before != nil {
			before(ctx)
		}
		tickChan := time.NewTicker(duration)
		defer tickChan.Stop()
		f(ctx, notifyStopped)
		for {
			select {
			case <-notifyStopped:
				t.Log.Info("exiting ticker task due to notifyStopped channel")
				return
			case <-ctx.Done():
				t.Log.Info("exiting ticker task due to stopped channel")
				return
			case <-tickChan.C:
				t.Log.Info("running ticker task again")
				f(ctx, notifyStopped)
			}
		}
	})
}
