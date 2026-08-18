package tasks

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func discardManager() *TaskManager {
	return NewTaskManager(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// Task replacement must cancel and await the stale AWS fetch.
func TestNewTaskCancelsPrevious(t *testing.T) {
	tm := discardManager()

	started := make(chan struct{})
	canceled := make(chan struct{})

	tm.NewTask(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(canceled)
	})

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first task never started")
	}

	tm.NewTask(func(ctx context.Context) {})

	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("previous task was not canceled when a new task started")
	}
}

func TestCloseNoCurrentTask(t *testing.T) {
	discardManager().Close()
}
