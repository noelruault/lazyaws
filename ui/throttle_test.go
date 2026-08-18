package ui

import (
	"testing"
	"time"
)

func TestThrottleCoalescesBurst(t *testing.T) {
	calls := make(chan struct{}, 10)
	th := newThrottle(30*time.Millisecond, func() { calls <- struct{}{} })

	// A burst collapses to one immediate call and one trailing call.
	for i := 0; i < 5; i++ {
		th.Trigger()
	}

	select {
	case <-calls:
	default:
		t.Fatal("expected an immediate call from the first Trigger")
	}

	select {
	case <-calls:
		t.Fatal("burst triggers inside the window should not fire immediately")
	case <-time.After(10 * time.Millisecond):
	}

	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("expected one trailing call after the window closed")
	}

	th.Trigger()
	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("expected an immediate call once idle")
	}
}
