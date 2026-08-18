package panels

// Ported from lazydocker's pkg/gui/panels/side_list_panel.go (MIT, © 2018 Jesse Duffield), adapted for lazyaws: go-errors -> stdlib errors, lazydocker/pkg/tasks -> ui/tasks, lazydocker/pkg/utils -> ui/utils.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/ui/tasks"
	"github.com/noelruault/lazyaws/ui/utils"
)

type ISideListPanel interface {
	SetMainTabIndex(int)
	HandleSelect() error
	GetView() *gocui.View
	Refocus()
	RerenderList() error
	IsFilterDisabled() bool
	IsHidden() bool
	HandleNextLine() error
	HandlePrevLine() error
	HandleClick() error
	HandlePrevMainTab() error
	HandleNextMainTab() error
	CurrentMainRows() *MainRows
}

type SideListPanel[T comparable] struct {
	ContextState *ContextState[T]

	ListPanel[T]

	NoItemsMessage string

	Gui IGui

	Filter func(T) bool
	Sort   func(a, b T) bool

	OnClick func(T) error

	OnSelect func(T) error

	GetTableCells func(T) []string

	OnRerender func() error

	DisableFilter bool

	Hide func() bool
}

var _ ISideListPanel = &SideListPanel[int]{}

type IGui interface {
	HandleClick(v *gocui.View, itemCount int, selectedLine *int, handleSelect func() error) error
	NewSimpleRenderStringTask(getContent func() string) tasks.TaskFunc
	FocusY(selectedLine int, itemCount int, view *gocui.View)
	ShouldRefresh(contextKey string) bool
	GetMainView() *gocui.View
	IsCurrentView(*gocui.View) bool
	FilterString(view *gocui.View) string
	IgnoreStrings() []string
	Update(func() error)

	QueueTask(f func(ctx context.Context)) error
}

func (self *SideListPanel[T]) HandleClick() error {
	itemCount := self.List.Len()
	handleSelect := self.HandleSelect
	selectedLine := &self.SelectedIdx

	if err := self.Gui.HandleClick(self.View, itemCount, selectedLine, handleSelect); err != nil {
		return err
	}

	if self.OnClick != nil {
		selectedItem, err := self.GetSelectedItem()
		if err == nil {
			return self.OnClick(selectedItem)
		}
	}

	return nil
}

func (self *SideListPanel[T]) GetView() *gocui.View {
	return self.View
}

func (self *SideListPanel[T]) HandleSelect() error {
	item, err := self.GetSelectedItem()
	if err != nil {
		if err.Error() != self.NoItemsMessage {
			return err
		}

		if self.NoItemsMessage != "" {
			self.Gui.NewSimpleRenderStringTask(func() string { return self.NoItemsMessage })
		}

		return nil
	}

	self.Refocus()

	if self.OnSelect != nil {
		if err := self.OnSelect(item); err != nil {
			return err
		}
	}

	return self.renderContext(item)
}

func (self *SideListPanel[T]) renderContext(item T) error {
	if self.ContextState == nil {
		return nil
	}

	key := self.ContextState.GetCurrentContextKey(item)
	if !self.Gui.ShouldRefresh(key) {
		return nil
	}

	mainView := self.Gui.GetMainView()
	mainView.Tabs = self.ContextState.GetMainTabTitles()
	mainView.TabIndex = self.ContextState.mainTabIdx

	task := self.ContextState.GetCurrentMainTab().Render(item)

	return self.Gui.QueueTask(task)
}

// CurrentMainRows reports the rows of the tab now showing, or nil when that tab is prose or nothing is selected.
// The generic tab callback is resolved here so the main panel can stay unaware of each panel's item type.
func (self *SideListPanel[T]) CurrentMainRows() *MainRows {
	if self.ContextState == nil {
		return nil
	}

	tab := self.ContextState.GetCurrentMainTab()
	if tab.Rows == nil {
		return nil
	}

	item, err := self.GetSelectedItem()
	if err != nil {
		return nil
	}

	return tab.Rows(item)
}

func (self *SideListPanel[T]) GetSelectedItem() (T, error) {
	var zero T

	item, ok := self.List.TryGet(self.SelectedIdx)
	if !ok {
		return zero, errors.New(self.NoItemsMessage)
	}

	return item, nil
}

// SelectByItem is for comparable identity; decorated rows must use SelectByCell.
func (self *SideListPanel[T]) SelectByItem(item T) bool {
	for idx, candidate := range self.List.GetItems() {
		if candidate == item {
			self.SetSelectedLineIdx(idx)
			return true
		}
	}

	return false
}

// SelectByCell matches rendered identity but cannot find decorated cells by raw name.
func (self *SideListPanel[T]) SelectByCell(needle string) bool {
	for idx, item := range self.List.GetItems() {
		for _, cell := range self.GetTableCells(item) {
			if cell == needle {
				self.SetSelectedLineIdx(idx)
				return true
			}
		}
	}

	return false
}

func (self *SideListPanel[T]) HandleNextLine() error {
	self.SelectNextLine()

	return self.HandleSelect()
}

func (self *SideListPanel[T]) HandlePrevLine() error {
	self.SelectPrevLine()

	return self.HandleSelect()
}

func (self *SideListPanel[T]) HandleNextMainTab() error {
	if self.ContextState == nil {
		return nil
	}

	self.ContextState.HandleNextMainTab()

	return self.HandleSelect()
}

func (self *SideListPanel[T]) HandlePrevMainTab() error {
	if self.ContextState == nil {
		return nil
	}

	self.ContextState.HandlePrevMainTab()

	return self.HandleSelect()
}

func (self *SideListPanel[T]) Refocus() {
	self.Gui.FocusY(self.SelectedIdx, self.List.Len(), self.View)
}

func (self *SideListPanel[T]) SetItems(items []T) {
	self.List.SetItems(items)
	self.FilterAndSort()
}

func (self *SideListPanel[T]) FilterAndSort() {
	filterString := self.Gui.FilterString(self.View)

	self.List.Filter(func(item T, index int) bool {
		if self.Filter != nil && !self.Filter(item) {
			return false
		}

		if slices.ContainsFunc(self.Gui.IgnoreStrings(), func(ignore string) bool {
			return slices.ContainsFunc(self.GetTableCells(item), func(searchString string) bool {
				return strings.Contains(searchString, ignore)
			})
		}) {
			return false
		}

		if filterString != "" {
			return slices.ContainsFunc(self.GetTableCells(item), func(searchString string) bool {
				return strings.Contains(searchString, filterString)
			})
		}

		return true
	})

	self.List.Sort(self.Sort)

	self.clampSelectedLineIdx()
}

func (self *SideListPanel[T]) RerenderList() error {
	self.FilterAndSort()

	self.Gui.Update(func() error {
		self.View.Clear()
		items := self.List.GetItems()
		table := make([][]string, len(items))
		for i, item := range items {
			table[i] = self.GetTableCells(item)
		}
		renderedTable, err := utils.RenderTable(table)
		if err != nil {
			return err
		}
		fmt.Fprint(self.View, renderedTable)

		if self.OnRerender != nil {
			if err := self.OnRerender(); err != nil {
				return err
			}
		}

		if self.Gui.IsCurrentView(self.View) {
			return self.HandleSelect()
		}
		return nil
	})

	return nil
}

func (self *SideListPanel[T]) SetMainTabIndex(index int) {
	if self.ContextState == nil {
		return
	}

	self.ContextState.SetMainTabIndex(index)
}

func (self *SideListPanel[T]) IsFilterDisabled() bool {
	return self.DisableFilter
}

func (self *SideListPanel[T]) IsHidden() bool {
	if self.Hide == nil {
		return false
	}

	return self.Hide()
}
