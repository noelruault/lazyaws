// Portions adapted from lazydocker's pkg/gui/view_helpers.go (MIT, © 2018 Jesse Duffield).
package ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/utils"
)

func (gui *Gui) handleGoTo(view *gocui.View) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		return gui.goToPanel(view)
	}
}

func (gui *Gui) nextView(g *gocui.Gui, v *gocui.View) error {
	sideViewNames := gui.sideViewNames()
	var focusedViewName string
	if v == nil || v.Name() == sideViewNames[len(sideViewNames)-1] {
		focusedViewName = sideViewNames[0]
	} else {
		viewName := v.Name()
		for i := range sideViewNames {
			if viewName == sideViewNames[i] {
				focusedViewName = sideViewNames[i+1]
				break
			}
			if i == len(sideViewNames)-1 {
				gui.Log.Info("not in list of views")
				return nil
			}
		}
	}
	focusedView, err := g.View(focusedViewName)
	if err != nil {
		panic(err)
	}
	gui.resetMainView()
	return gui.switchFocus(focusedView)
}

func (gui *Gui) previousView(g *gocui.Gui, v *gocui.View) error {
	sideViewNames := gui.sideViewNames()
	var focusedViewName string
	if v == nil || v.Name() == sideViewNames[0] {
		focusedViewName = sideViewNames[len(sideViewNames)-1]
	} else {
		viewName := v.Name()
		for i := range sideViewNames {
			if viewName == sideViewNames[i] {
				focusedViewName = sideViewNames[i-1]
				break
			}
			if i == len(sideViewNames)-1 {
				gui.Log.Info("not in list of views")
				return nil
			}
		}
	}
	focusedView, err := g.View(focusedViewName)
	if err != nil {
		panic(err)
	}
	gui.resetMainView()
	return gui.switchFocus(focusedView)
}

func (gui *Gui) resetMainView() {
	gui.State.Panels.Main.ObjectKey = ""
	gui.Views.Main.Wrap = gui.Config.User.Gui.WrapMainPanel
}

// nolint:unparam
func (gui *Gui) focusPoint(selectedX int, selectedY int, lineCount int, v *gocui.View) {
	if selectedY < 0 || selectedY > lineCount {
		return
	}
	ox, oy := v.Origin()
	originalOy := oy
	cx, cy := v.Cursor()
	originalCy := cy
	_, height := v.Size()

	ly := max(height-1, 0)

	windowStart := oy
	windowEnd := oy + ly

	if selectedY < windowStart {
		oy = max(oy-(windowStart-selectedY), 0)
	} else if selectedY > windowEnd {
		oy += (selectedY - windowEnd)
	}

	if windowEnd > lineCount-1 {
		shiftAmount := (windowEnd - (lineCount - 1))
		oy = max(oy-shiftAmount, 0)
	}

	if originalOy != oy {
		_ = v.SetOrigin(ox, oy)
	}

	cy = selectedY - oy
	if originalCy != cy {
		_ = v.SetCursor(cx, selectedY-oy)
	}
}

func (gui *Gui) FocusY(selectedY int, lineCount int, v *gocui.View) {
	gui.focusPoint(0, selectedY, lineCount, v)
}

func (gui *Gui) ResetOrigin(v *gocui.View) {
	_ = v.SetOrigin(0, 0)
	_ = v.SetCursor(0, 0)
}

func (gui *Gui) cleanString(s string) string {
	// A leading UTF-8 BOM would render as a stray glyph; only the first is a marker, any later one is content.
	return utils.NormalizeLinefeeds(strings.TrimPrefix(s, "\uFEFF"))
}

func (gui *Gui) setViewContent(v *gocui.View, s string) error {
	v.Clear()
	fmt.Fprint(v, gui.cleanString(s))
	return nil
}

func (gui *Gui) renderString(g *gocui.Gui, viewName, s string) error {
	g.Update(func(*gocui.Gui) error {
		v, err := g.View(viewName)
		if err != nil {
			return nil
		}
		if err := v.SetOrigin(0, 0); err != nil {
			return err
		}
		if err := v.SetCursor(0, 0); err != nil {
			return err
		}
		return gui.setViewContent(v, s)
	})
	return nil
}

func (gui *Gui) RenderStringMain(s string) {
	gui.g.Update(func(*gocui.Gui) error {
		if gui.mainBelongsToQ() {
			return nil
		}
		v, err := gui.g.View("main")
		if err != nil {
			return nil
		}
		if err := v.SetOrigin(0, 0); err != nil {
			return err
		}
		if err := v.SetCursor(0, 0); err != nil {
			return err
		}
		return gui.setViewContent(v, s)
	})
}

func (gui *Gui) reRenderStringMain(s string) {
	gui.g.Update(gui.setMainContent(s))
}

// reRenderStringMainOrdered is reRenderStringMain with the enqueue done on THIS goroutine: gocui's Update spawns a goroutine per call, so two writes made back to back can apply in either order, and a pane painted "loading" before its content would sometimes keep the loading line.
// Only worth reaching for when one goroutine writes main twice in a row; a single write has nothing to race with.
func (gui *Gui) reRenderStringMainOrdered(s string) {
	gui.g.UpdateAsync(gui.setMainContent(s))
}

func (gui *Gui) setMainContent(s string) func(*gocui.Gui) error {
	return func(*gocui.Gui) error {
		if gui.mainBelongsToQ() {
			return nil
		}
		v, err := gui.g.View("main")
		if err != nil {
			return nil
		}
		return gui.setViewContent(v, s)
	}
}

// mainBelongsToQ checks queued updates so stale chat work cannot overwrite the dashboard.
func (gui *Gui) mainBelongsToQ() bool {
	return gui.State.Q != nil && gui.qScreenActive()
}

// streamStringMain uses UpdateAsync so older partial renders cannot land last.
func (gui *Gui) streamStringMain(s string) {
	gui.g.UpdateAsync(func(*gocui.Gui) error {
		if !gui.mainBelongsToQ() {
			return nil
		}
		v, err := gui.g.View("main")
		if err != nil {
			return nil
		}
		return gui.setViewContent(v, s)
	})
}

func (gui *Gui) reRenderString(viewName, s string) {
	gui.g.Update(func(*gocui.Gui) error {
		v, err := gui.g.View(viewName)
		if err != nil {
			return nil
		}
		return gui.setViewContent(v, s)
	})
}

func (gui *Gui) optionsMapToString(optionsMap map[string]string) string {
	// Sorted before colouring: an ANSI prefix would sort every entry by escape byte instead of keycap.
	keys := make([]string, 0, len(optionsMap))
	for key := range optionsMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	optionsArray := make([]string, len(keys))
	for i, key := range keys {
		optionsArray[i] = utils.ColoredString(key, color.FgCyan) + " " + capitalized(optionsMap[key])
	}
	return strings.Join(optionsArray, optionsSeparator)
}

func (gui *Gui) renderOptionsMap(optionsMap map[string]string) error {
	return gui.renderString(gui.g, "options", gui.optionsMapToString(optionsMap))
}

// option is one entry of an ORDERED options line, which the popups' alphabetical map cannot express.
type option struct {
	key   string
	label string
}

// optionsSeparator is the three-space gap the redesign mockups put between footer entries; the journeys split the footer on it, so the two must move together.
const optionsSeparator = "   "

// capitalized uppercases only the first rune, so labels stay lowercase where they are declared and the mockup's Title Case is a rendering concern.
func capitalized(label string) string {
	runes := []rune(label)
	if len(runes) == 0 {
		return label
	}
	return string(unicode.ToUpper(runes[0])) + string(runes[1:])
}

// optionsToString reads the line in the order given, so the first thing cut when the terminal is narrow is the last thing listed.
func optionsToString(options []option) string {
	parts := make([]string, len(options))
	for i, opt := range options {
		parts[i] = utils.ColoredString(opt.key, color.FgCyan) + " " + capitalized(opt.label)
	}

	return strings.Join(parts, optionsSeparator)
}

func (gui *Gui) GetMainView() *gocui.View {
	return gui.Views.Main
}

func (gui *Gui) trimmedContent(v *gocui.View) string {
	return strings.TrimSpace(v.Buffer())
}

func (gui *Gui) currentViewName() string {
	currentView := gui.g.CurrentView()
	if currentView == nil {
		return gui.initiallyFocusedViewName()
	}
	return currentView.Name()
}

func (gui *Gui) resizeCurrentPopupPanel(g *gocui.Gui) error {
	v := g.CurrentView()
	// No focused view yet: the layout pass can run before the initial switchFocus, same window currentViewName() guards against.
	if v == nil {
		return nil
	}
	if gui.isPopupPanel(v.Name()) {
		return gui.resizePopupPanel(v)
	}
	return nil
}

func (gui *Gui) resizePopupPanel(v *gocui.View) error {
	content := v.Buffer()
	x0, y0, x1, y1 := gui.getConfirmationPanelDimensions(v.Wrap, content)
	vx0, vy0, vx1, vy1 := v.Dimensions()
	if vx0 == x0 && vy0 == y0 && vx1 == x1 && vy1 == y1 {
		return nil
	}
	_, err := gui.g.SetView(v.Name(), x0, y0, x1, y1, 0)
	return err
}

func (gui *Gui) renderPanelOptions() error {
	currentView := gui.g.CurrentView()
	switch currentView.Name() {
	case "menu":
		return gui.renderMenuOptions()
	case "confirmation":
		return gui.renderConfirmationOptions()
	case "qInput", "qChats":
		return gui.renderQOptions()
	case "settings":
		return gui.renderSettingsOptions()
	}
	return gui.renderGlobalOptions()
}

// clearMainView queues through gocui because ticker hooks run off the UI thread.
func (gui *Gui) clearMainView() {
	gui.g.Update(func(*gocui.Gui) error {
		mainView := gui.Views.Main
		mainView.Clear()
		_ = mainView.SetOrigin(0, 0)
		_ = mainView.SetCursor(0, 0)
		return nil
	})
}

func (gui *Gui) nextScreenMode() error {
	if gui.currentViewName() == "main" {
		gui.State.ScreenMode = prevIntInCycle([]WindowMaximisation{SCREEN_NORMAL, SCREEN_HALF, SCREEN_FULL}, gui.State.ScreenMode)
		return nil
	}

	gui.State.ScreenMode = nextIntInCycle([]WindowMaximisation{SCREEN_NORMAL, SCREEN_HALF, SCREEN_FULL}, gui.State.ScreenMode)
	return nil
}

func (gui *Gui) prevScreenMode() error {
	if gui.currentViewName() == "main" {
		gui.State.ScreenMode = nextIntInCycle([]WindowMaximisation{SCREEN_NORMAL, SCREEN_HALF, SCREEN_FULL}, gui.State.ScreenMode)
		return nil
	}

	gui.State.ScreenMode = prevIntInCycle([]WindowMaximisation{SCREEN_NORMAL, SCREEN_HALF, SCREEN_FULL}, gui.State.ScreenMode)
	return nil
}

func nextIntInCycle(sl []WindowMaximisation, current WindowMaximisation) WindowMaximisation {
	for i, val := range sl {
		if val == current {
			if i == len(sl)-1 {
				return sl[0]
			}
			return sl[i+1]
		}
	}
	return sl[0]
}

func prevIntInCycle(sl []WindowMaximisation, current WindowMaximisation) WindowMaximisation {
	for i, val := range sl {
		if val == current {
			if i > 0 {
				return sl[i-1]
			}
			return sl[len(sl)-1]
		}
	}
	return sl[len(sl)-1]
}

func (gui *Gui) HandleClick(v *gocui.View, itemCount int, selectedLine *int, handleSelect func() error) error {
	wrappedHandleSelect := func(g *gocui.Gui, v *gocui.View) error {
		return handleSelect()
	}
	return gui.handleClickAux(v, itemCount, selectedLine, wrappedHandleSelect)
}

func (gui *Gui) handleClickAux(v *gocui.View, itemCount int, selectedLine *int, handleSelect func(*gocui.Gui, *gocui.View) error) error {
	if gui.popupPanelFocused() && v != nil && !gui.isPopupPanel(v.Name()) {
		return nil
	}

	_, cy := v.Cursor()
	_, oy := v.Origin()

	newSelectedLine := cy + oy

	if newSelectedLine < 0 {
		newSelectedLine = 0
	}

	if newSelectedLine > itemCount-1 {
		newSelectedLine = itemCount - 1
	}

	*selectedLine = newSelectedLine

	if gui.currentViewName() != v.Name() {
		if err := gui.switchFocus(v); err != nil {
			return err
		}
	}

	return handleSelect(gui.g, v)
}

func (gui *Gui) CurrentView() *gocui.View {
	return gui.g.CurrentView()
}

func (gui *Gui) sidePanelNamed(viewName string) (panels.ISideListPanel, bool) {
	for _, sidePanel := range gui.allSidePanels() {
		if sidePanel.GetView().Name() == viewName {
			return sidePanel, true
		}
	}

	return nil, false
}

func (gui *Gui) currentSidePanel() (panels.ISideListPanel, bool) {
	return gui.sidePanelNamed(gui.currentViewName())
}

// sidePanelForMain resolves through focus history so detail tabs survive main focus.
func (gui *Gui) sidePanelForMain() (panels.ISideListPanel, bool) {
	return gui.sidePanelNamed(gui.currentSideViewName())
}

func (gui *Gui) currentListPanel() (panels.ISideListPanel, bool) {
	viewName := gui.currentViewName()

	for _, sidePanel := range gui.allListPanels() {
		if sidePanel.GetView().Name() == viewName {
			return sidePanel, true
		}
	}

	return nil, false
}

func (gui *Gui) allSidePanels() []panels.ISideListPanel {
	return []panels.ISideListPanel{
		gui.Panels.Profile,
		gui.Panels.ECS,
		gui.Panels.EC2,
		gui.Panels.S3,
		gui.Panels.EKS,
		gui.Panels.ECR,
		gui.Panels.Secrets,
		gui.Panels.VPC,
	}
}

func (gui *Gui) allListPanels() []panels.ISideListPanel {
	return append(gui.allSidePanels(), gui.Panels.Menu)
}

func (gui *Gui) IsCurrentView(view *gocui.View) bool {
	return view == gui.CurrentView()
}
