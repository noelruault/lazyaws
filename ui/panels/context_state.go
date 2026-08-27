package panels

// Ported from lazydocker's pkg/gui/panels/context_state.go (MIT, © 2018 Jesse Duffield), adapted for lazyaws: lazydocker/pkg/tasks -> ui/tasks.

import (
	"github.com/noelruault/lazyaws/ui/tasks"
)

type ContextState[T any] struct {
	mainTabIdx  int
	GetMainTabs func() []MainTab[T]
	// The key decides when the main panel re-renders: include the item's ID, plus anything else that should invalidate the cache (e.g. the container's state).
	GetItemContextCacheKey func(item T) string
}

type MainTab[T any] struct {
	Key    string
	Title  string
	Render func(item T) tasks.TaskFunc
	// Rows opts the tab into main-panel navigation; see MainRows. Nil leaves it a prose tab.
	Rows func(item T) *MainRows
}

func (self *ContextState[T]) GetMainTabTitles() []string {
	tabs := self.GetMainTabs()
	titles := make([]string, len(tabs))
	for i, tab := range tabs {
		titles[i] = tab.Title
	}

	return titles
}

func (self *ContextState[T]) GetCurrentContextKey(item T) string {
	return self.GetItemContextCacheKey(item) + "-" + self.GetCurrentMainTab().Key
}

// GetCurrentMainTab falls back to the first tab when the set has shrunk under the index.
// A panel's tab set can change while an index is held (the ECS panel's differs per drill level), and not every path that changes it resets the index, so the previous set's last tab reads past the end of the new one.
func (self *ContextState[T]) GetCurrentMainTab() MainTab[T] {
	tabs := self.GetMainTabs()
	if self.mainTabIdx >= len(tabs) {
		self.mainTabIdx = 0
	}

	return tabs[self.mainTabIdx]
}

func (self *ContextState[T]) HandleNextMainTab() {
	tabs := self.GetMainTabs()

	if len(tabs) == 0 {
		return
	}

	self.mainTabIdx = (self.mainTabIdx + 1) % len(tabs)
}

func (self *ContextState[T]) HandlePrevMainTab() {
	tabs := self.GetMainTabs()

	if len(tabs) == 0 {
		return
	}

	self.mainTabIdx = (self.mainTabIdx - 1 + len(tabs)) % len(tabs)
}

func (self *ContextState[T]) SetMainTabIndex(index int) {
	self.mainTabIdx = index
}
