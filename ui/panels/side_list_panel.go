package panels

// Ported from lazydocker's pkg/gui/panels/side_list_panel.go (MIT, © 2018 Jesse Duffield), adapted for lazyaws: go-errors -> stdlib errors, lazydocker/pkg/tasks -> ui/tasks, lazydocker/pkg/utils -> ui/utils.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/fatih/color"
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
	SelectedCopyValue() (string, bool)
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

	// GetTableCellsFit renders a row as plain-text cells, laying the panel out with RenderTableFit instead of RenderTable so one long value cannot push the other columns off-screen.
	// A panel sets this or GetTableCells, never both; the panels still on GetTableCells are the ones stage 2 has not migrated yet.
	GetTableCellsFit func(T) []utils.Cell
	// CopyValue answers the full, untruncated identifier of a row for the copy popup.
	// It is deliberately not the selection key: the key must stay something a reload can match on, while this is the value a user pastes elsewhere, so a resource carrying an ARN reports the ARN.
	CopyValue func(T) string
	// Weights sizes the columns of a table whose rows are shaped like the one passed, which is the first row on screen.
	// It takes an item because a panel can change row shape as it is used (the ECS panel's rows differ per drill level), and reading the shape off the rows actually being rendered is what stops the two from drifting apart.
	Weights func(T) []int

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

// SelectedCopyValue reports false rather than an empty string so a caller cannot open a popup showing nothing: a panel with no CopyValue, no rows, or a row whose identifier the list call left blank all mean "there is nothing to copy here".
func (self *SideListPanel[T]) SelectedCopyValue() (string, bool) {
	if self.CopyValue == nil {
		return "", false
	}

	item, err := self.GetSelectedItem()
	if err != nil {
		return "", false
	}

	value := self.CopyValue(item)
	return value, value != ""
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
		for _, cell := range self.searchCells(item) {
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

// SetItemsKeepSelection replaces the rows and keeps the selection on the same resource rather than on the same line.
// The panels sort running-first, so any reload that changes one item's state reorders the list and an index-preserved selection silently lands on a different resource, with the detail pane then describing something other than the highlighted row.
// key must be an identity, not a cache key: ContextState.GetItemContextCacheKey deliberately mixes in mutable state (the secrets panel folds in whether the value is revealed), so it is the wrong thing to match on here.
func (self *SideListPanel[T]) SetItemsKeepSelection(items []T, key func(T) string) {
	previous := ""
	if item, err := self.GetSelectedItem(); err == nil {
		previous = key(item)
	}

	self.SetItems(items)

	// An empty key cannot identify anything, so an item that has none leaves the clamped index alone rather than matching the first other item that also has none.
	if previous == "" {
		return
	}

	for idx, item := range self.List.GetItems() {
		if key(item) == previous {
			self.SetSelectedLineIdx(idx)
			return
		}
	}
}

func (self *SideListPanel[T]) FilterAndSort() {
	filterString := self.Gui.FilterString(self.View)

	self.List.Filter(func(item T, index int) bool {
		if self.Filter != nil && !self.Filter(item) {
			return false
		}

		if slices.ContainsFunc(self.Gui.IgnoreStrings(), func(ignore string) bool {
			return slices.ContainsFunc(self.searchCells(item), func(searchString string) bool {
				return strings.Contains(searchString, ignore)
			})
		}) {
			return false
		}

		if filterString != "" {
			return slices.ContainsFunc(self.searchCells(item), func(searchString string) bool {
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
		if len(items) == 0 {
			fmt.Fprint(self.View, self.emptyMessage())
			return self.afterRerender()
		}

		renderedTable, err := self.renderTable(items)
		if err != nil {
			return err
		}
		fmt.Fprint(self.View, renderedTable)

		return self.afterRerender()
	})

	return nil
}

// renderTable lays the rows out for the view's current width, which is why it must run inside the Update closure rather than ahead of it.
func (self *SideListPanel[T]) renderTable(items []T) (string, error) {
	if self.GetTableCellsFit == nil {
		table := make([][]string, len(items))
		for i, item := range items {
			table[i] = self.GetTableCells(item)
		}

		return utils.RenderTable(table)
	}

	table := make([][]utils.Cell, len(items))
	for i, item := range items {
		table[i] = self.GetTableCellsFit(item)
	}

	return utils.RenderTableFit(table, self.View.InnerWidth(), self.Weights(items[0]))
}

// searchCells is the plain text of a row, for filtering and for finding a row by what it says.
// Cell.Text is unstyled by construction, whereas GetTableCells hands back strings that already carry colour escapes, so a filter over those can only ever match the columns nothing colours.
func (self *SideListPanel[T]) searchCells(item T) []string {
	if self.GetTableCellsFit == nil {
		return self.GetTableCells(item)
	}

	cells := self.GetTableCellsFit(item)
	texts := make([]string, len(cells))
	for i, cell := range cells {
		texts[i] = cell.Text
	}

	return texts
}

// emptyMessage is what a panel shows in place of rows.
// HandleSelect puts the same words in the main panel, but only for the focused panel, so without this an empty side panel is an unexplained blank box.
// It is a method rather than an inline call so the muting is assertable: gocui parses escapes into cell attributes, and View.Buffer() hands the text back stripped of them.
func (self *SideListPanel[T]) emptyMessage() string {
	if self.NoItemsMessage == "" {
		return ""
	}

	return utils.ColoredString(self.NoItemsMessage, color.Faint)
}

// afterRerender runs the hooks every rerender owes its caller, whether or not the panel had rows to draw.
func (self *SideListPanel[T]) afterRerender() error {
	if self.OnRerender != nil {
		if err := self.OnRerender(); err != nil {
			return err
		}
	}

	if self.Gui.IsCurrentView(self.View) {
		return self.HandleSelect()
	}
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
