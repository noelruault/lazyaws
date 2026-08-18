package ui

import (
	"sync"
	"testing"
	"time"
)

func TestStatusManagerStack(t *testing.T) {
	m := &statusManager{}
	if got := m.getStatusString(); got != "" {
		t.Fatalf("empty manager: got %q, want \"\"", got)
	}

	m.addWaitingStatus("first")
	m.addWaitingStatus("second")
	if got := m.getStatusString(); got[:len("second ")] != "second " {
		t.Fatalf("top status: got %q, want prefix %q", got, "second ")
	}

	m.addWaitingStatus("first")
	if got := m.getStatusString(); got[:len("first ")] != "first " {
		t.Fatalf("re-added status not on top: got %q", got)
	}
	if len(m.statuses) != 2 {
		t.Fatalf("re-add duplicated instead of dedup: len=%d, want 2", len(m.statuses))
	}

	m.removeStatus("first")
	m.removeStatus("second")
	if got := m.getStatusString(); got != "" {
		t.Fatalf("after removing all: got %q, want \"\"", got)
	}
}

// The final blank render erases the bottom-line status.
func TestStatusManagerSpinClearsOnDrain(t *testing.T) {
	m := &statusManager{}
	m.addWaitingStatus("switching profile")

	ticks := make(chan time.Time)
	var rendered []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.spin(ticks, func(s string) { rendered = append(rendered, s) })
	}()

	ticks <- time.Time{}
	m.removeStatus("switching profile")
	ticks <- time.Time{}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("spin did not return after the status stack drained")
	}

	if len(rendered) != 2 {
		t.Fatalf("renders = %q, want 2 (one status, one clearing blank)", rendered)
	}
	if rendered[0][:len("switching profile ")] != "switching profile " {
		t.Errorf("first render = %q, want prefix %q", rendered[0], "switching profile ")
	}
	if rendered[1] != "" {
		t.Errorf("last render = %q, want \"\" so the status bar is erased", rendered[1])
	}
}

// Concurrent status writers and the spinner reader must remain race-free.
func TestStatusManagerConcurrentAccess(t *testing.T) {
	m := &statusManager{}
	var wg sync.WaitGroup

	for i := range 8 {
		wg.Go(func() {
			name := string(rune('a' + i))
			for range 50 {
				m.addWaitingStatus(name)
				_ = m.getStatusString()
				m.removeStatus(name)
			}
		})
	}

	wg.Wait()
	if got := m.getStatusString(); got != "" {
		t.Fatalf("after all goroutines removed their status: got %q, want \"\"", got)
	}
}
