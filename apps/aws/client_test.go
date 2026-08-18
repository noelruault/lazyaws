package aws

import (
	"context"
	"testing"
	"time"
)

func TestWithDefaultTimeoutPreservesParentDeadline(t *testing.T) {
	want := time.Now().Add(time.Minute)
	parentCtx, parentCancel := context.WithDeadline(context.Background(), want)
	defer parentCancel()

	timeoutCtx, cancel := withDefaultTimeout(parentCtx, time.Second)
	defer cancel()

	got, ok := timeoutCtx.Deadline()
	if !ok {
		t.Fatal("withDefaultTimeout() removed the parent deadline")
	}
	if !got.Equal(want) {
		t.Fatalf("withDefaultTimeout() deadline = %v, want %v", got, want)
	}
}

func TestWithDefaultTimeoutAddsCancellableDeadline(t *testing.T) {
	started := time.Now()
	timeoutCtx, cancel := withDefaultTimeout(context.Background(), time.Second)

	deadline, ok := timeoutCtx.Deadline()
	if !ok {
		t.Fatal("withDefaultTimeout() did not add a deadline")
	}
	if deadline.Before(started.Add(900*time.Millisecond)) || deadline.After(started.Add(2*time.Second)) {
		t.Fatalf("withDefaultTimeout() deadline = %v, want roughly one second after %v", deadline, started)
	}

	cancel()
	select {
	case <-timeoutCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("withDefaultTimeout() cancel did not stop the context")
	}
}
