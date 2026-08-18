// Package ui coalesces bursty keys and profile switches to bound AWS reloads.
package ui

import (
	"sync"
	"time"
)

type throttle struct {
	mu       sync.Mutex
	interval time.Duration
	fn       func()
	timer    *time.Timer
	pending  bool
}

func newThrottle(interval time.Duration, fn func()) *throttle {
	return &throttle{interval: interval, fn: fn}
}

func (t *throttle) Trigger() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.timer != nil {
		t.pending = true
		return
	}

	t.fn()
	t.timer = time.AfterFunc(t.interval, t.fire)
}

func (t *throttle) fire() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.timer = nil
	if t.pending {
		t.pending = false
		t.fn()
		t.timer = time.AfterFunc(t.interval, t.fire)
	}
}
