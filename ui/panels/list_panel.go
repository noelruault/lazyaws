package panels

import (
	"github.com/jesseduffield/gocui"
)

type ListPanel[T comparable] struct {
	SelectedIdx int
	List        *FilteredList[T]
	View        *gocui.View
}

func (self *ListPanel[T]) SetSelectedLineIdx(value int) {
	clampedValue := 0
	if self.List.Len() > 0 {
		clampedValue = clamp(value, 0, self.List.Len()-1)
	}

	self.SelectedIdx = clampedValue
}

func (self *ListPanel[T]) clampSelectedLineIdx() {
	clamped := clamp(self.SelectedIdx, 0, self.List.Len()-1)

	if clamped != self.SelectedIdx {
		self.SelectedIdx = clamped
	}
}

func (self *ListPanel[T]) moveSelectedLine(delta int) {
	self.SetSelectedLineIdx(self.SelectedIdx + delta)
}

func (self *ListPanel[T]) SelectNextLine() {
	self.moveSelectedLine(1)
}

func (self *ListPanel[T]) SelectPrevLine() {
	self.moveSelectedLine(-1)
}

// clamp keeps the guard order the panels rely on: on an empty list the range inverts, and a value below lo must still win.
func clamp(value, lo, hi int) int {
	if value < lo {
		return lo
	}
	if value > hi {
		return hi
	}

	return value
}
