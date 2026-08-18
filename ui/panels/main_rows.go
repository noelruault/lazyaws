package panels

// MainRows makes a main-panel tab navigable: the pane gets a cursor, and Enter and the actions key address the row under it. A tab that leaves MainTab.Rows nil stays prose, and the same keys keep scrolling the pane instead.
type MainRows struct {
	// Header sits above the rows and is not addressable; a blank one is omitted along with its spacing.
	Header string
	// EmptyMessage replaces the table when there are no rows, so an empty tab still says which tab it is.
	EmptyMessage string
	Cells        [][]string

	// Enter drills into row i. Nil means the rows are terminal and Enter does nothing.
	Enter func(i int) error
	// Actions opens row i's affordance, whether that is a menu or an action list; the tab decides which.
	Actions func(i int) error
	// Back leaves the current level, for tabs that drill. Nil returns focus to the side panel instead.
	Back func() error
}

func (r *MainRows) Len() int {
	if r == nil {
		return 0
	}
	return len(r.Cells)
}
