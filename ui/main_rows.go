package ui

import (
	"strings"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/utils"
)

// mainCursorState is keyed by the main panel's context key so the cursor survives a re-render of the same list but resets whenever the selected item or tab changes underneath it.
type mainCursorState struct {
	key   string
	index int
}

// activeMainRows reports the navigable rows of the tab now showing, if it has any.
// Resolving through sidePanelForMain rather than the focused view keeps this working once focus has moved into main.
func (gui *Gui) activeMainRows() (*panels.MainRows, bool) {
	panel, ok := gui.sidePanelForMain()
	if !ok {
		return nil, false
	}

	rows := panel.CurrentMainRows()
	if rows == nil {
		return nil, false
	}

	return rows, true
}

// mainCursor returns the cursor for the list on screen, restarting at zero when a different list has taken its place.
func (gui *Gui) mainCursor(rows *panels.MainRows) int {
	key := gui.State.Panels.Main.ObjectKey
	if gui.mainCursorState.key != key {
		gui.mainCursorState = mainCursorState{key: key}
	}

	return clampCursor(gui.mainCursorState.index, rows.Len())
}

func (gui *Gui) setMainCursor(index int) {
	gui.mainCursorState = mainCursorState{key: gui.State.Panels.Main.ObjectKey, index: index}
}

// clampCursor keeps the cursor inside a list that may have shrunk since it was last moved.
func clampCursor(index, length int) int {
	if length == 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= length {
		return length - 1
	}
	return index
}

// renderMainRows draws the rows with the cursor line marked. The main view has no per-line selection primitive, so selection is baked into the text and the whole block is re-rendered on every move.
func renderMainRows(rows *panels.MainRows, cursor int) string {
	header := ""
	if rows.Header != "" {
		header = rows.Header + "\n\n"
	}

	if len(rows.Cells) == 0 {
		return header + rows.EmptyMessage + "\n"
	}

	table, err := utils.RenderTable(rows.Cells)
	if err != nil {
		return header + err.Error()
	}

	var b strings.Builder
	b.WriteString(header)
	for i, line := range strings.Split(table, "\n") {
		if i == cursor {
			b.WriteString(utils.ColoredString("> "+line, color.FgCyan) + "\n")
			continue
		}
		b.WriteString("  " + line + "\n")
	}

	return b.String()
}

func (gui *Gui) moveMainCursor(rows *panels.MainRows, delta int) error {
	cursor := clampCursor(gui.mainCursor(rows)+delta, rows.Len())
	gui.setMainCursor(cursor)

	content := renderMainRows(rows, cursor)
	gui.reRenderStringMain(content)
	gui.focusMainCursor(content, cursor)

	return nil
}

// focusMainCursor scrolls the pane so the cursor line stays on screen, offsetting by the unaddressable header above the rows.
func (gui *Gui) focusMainCursor(content string, cursor int) {
	gui.g.Update(func(g *gocui.Gui) error {
		view, err := g.View("main")
		if err != nil {
			return nil
		}

		lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
		gui.FocusY(cursor+mainRowsHeaderOffset(content), len(lines), view)
		return nil
	})
}

// mainRowsHeaderOffset counts the lines renderMainRows puts above row zero, so a cursor index maps to a screen line.
func mainRowsHeaderOffset(content string) int {
	// The header, when present, is followed by a blank line; both sit above the first row.
	if idx := strings.Index(content, "\n\n"); idx >= 0 {
		return strings.Count(content[:idx], "\n") + 2
	}
	return 0
}

// navigableMainRows reports whether the arrow keys should move a cursor instead of scrolling the pane.
// Two distinct cases have to scroll: a prose tab, which supplies no rows at all, and a drilled-in detail view, which supplies rows with nothing in them to walk.
func (gui *Gui) navigableMainRows() (*panels.MainRows, bool) {
	rows, ok := gui.activeMainRows()
	if !ok || rows.Len() == 0 {
		return nil, false
	}
	return rows, true
}

func (gui *Gui) handleMainUp() error {
	if rows, ok := gui.navigableMainRows(); ok {
		return gui.moveMainCursor(rows, -1)
	}
	return gui.scrollUpMain()
}

func (gui *Gui) handleMainDown() error {
	if rows, ok := gui.navigableMainRows(); ok {
		return gui.moveMainCursor(rows, 1)
	}
	return gui.scrollDownMain()
}

func (gui *Gui) handleMainEnter() error {
	rows, ok := gui.activeMainRows()
	if !ok || rows.Enter == nil || rows.Len() == 0 {
		return nil
	}
	return rows.Enter(gui.mainCursor(rows))
}

func (gui *Gui) handleMainAction() error {
	rows, ok := gui.activeMainRows()
	if !ok || rows.Actions == nil || rows.Len() == 0 {
		return nil
	}
	return rows.Actions(gui.mainCursor(rows))
}

// handleMainEscape climbs one level in a drilling tab before it will leave the panel.
func (gui *Gui) handleMainEscape(g *gocui.Gui, v *gocui.View) error {
	if rows, ok := gui.activeMainRows(); ok && rows.Back != nil {
		return rows.Back()
	}
	return gui.handleExitMain(g, v)
}
