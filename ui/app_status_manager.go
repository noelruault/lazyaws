// Ported from lazydocker's pkg/gui/app_status_manager.go (MIT, © 2018 Jesse Duffield), adapted for lazyaws.
package ui

import (
	"sync"
	"time"

	"github.com/noelruault/lazyaws/ui/utils"
)

type appStatus struct {
	name       string
	statusType string
	duration   int
}

type statusManager struct {
	// One mutex is sufficient because the status stack remains tiny.
	mu       sync.Mutex
	statuses []appStatus
}

func (m *statusManager) removeStatus(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeStatusLocked(name)
}

func (m *statusManager) removeStatusLocked(name string) {
	newStatuses := []appStatus{}
	for _, status := range m.statuses {
		if status.name != name {
			newStatuses = append(newStatuses, status)
		}
	}
	m.statuses = newStatuses
}

func (m *statusManager) addWaitingStatus(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.removeStatusLocked(name)
	newStatus := appStatus{
		name:       name,
		statusType: "waiting",
		duration:   0,
	}
	m.statuses = append([]appStatus{newStatus}, m.statuses...)
}

func (m *statusManager) getStatusString() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.statuses) == 0 {
		return ""
	}
	topStatus := m.statuses[0]
	if topStatus.statusType == "waiting" {
		return topStatus.name + " " + utils.Loader()
	}
	return topStatus.name
}

// spin must render empty before returning or the final spinner frame remains frozen.
func (m *statusManager) spin(ticks <-chan time.Time, render func(string)) {
	for range ticks {
		status := m.getStatusString()
		render(status)
		if status == "" {
			return
		}
	}
}

func (gui *Gui) WithWaitingStatus(name string, f func() error) error {
	go func() {
		gui.statusManager.addWaitingStatus(name)

		defer func() {
			gui.statusManager.removeStatus(name)
		}()

		go func() {
			ticker := time.NewTicker(time.Millisecond * 50)
			defer ticker.Stop()
			gui.statusManager.spin(ticker.C, func(appStatus string) {
				if err := gui.renderString(gui.g, "appStatus", appStatus); err != nil {
					gui.Log.Warn(err.Error())
				}
			})
		}()

		// ErrorChan keeps background status work from mutating popups directly.
		if err := f(); err != nil {
			gui.ErrorChan <- err
		}
	}()

	return nil
}

func (gui *Gui) getInformationContent() string {
	return gui.Version
}

func (gui *Gui) renderGlobalOptions() error {
	return gui.renderString(gui.g, "options", optionsToString(gui.dashboardOptions(gui.currentViewName())))
}

// dashboardOptions is the options line for the eight lists and the main pane, in reading order rather than alphabetically by keycap.
// Every rebindable label resolves through the keymap, so a user who moves a key sees the key they bound; the arrow, tab and page keys are literals no config can move, which is why those keycaps are written out.
// It is deliberately shorter than the set of keys that work here: the line is one row of a shared bottom line, and the menu (x / ?) is where the full list lives.
func (gui *Gui) dashboardOptions(viewName string) []option {
	named := func(name KeyName, label string) option {
		return option{key: describeKey(gui.Keys.Get(name).Key), label: label}
	}

	panel, isList := gui.sidePanelNamed(viewName)
	onMain := viewName == "main"

	// The chat screen hands focus to main, and there the dashboard's keys do nothing: the lists are hidden, so there is no selection to inspect, copy or act on, and main holds a conversation rather than tabs.
	if onMain && gui.mainBelongsToQ() {
		return []option{
			{key: "←→↑↓", label: "scroll"},
			{key: "tab", label: "next pane"},
			{key: "esc", label: "dashboard"},
			named(KeyQuit, "quit"),
		}
	}

	options := []option{{key: "←→↑↓", label: "navigate"}}
	if onMain {
		options = []option{{key: "←→↑↓", label: "scroll"}, {key: "[ ]", label: "tabs"}}
	}

	switch {
	case onMain:
		options = append(options, option{key: "enter", label: "select"})
	case viewName == "profile":
		options = append(options, option{key: "enter", label: "switch"})
	case viewName == "ecs":
		options = append(options, option{key: "enter", label: "drill down"})
	case isList:
		options = append(options, option{key: "enter", label: "inspect"})
	}

	if isList || onMain {
		options = append(options, named(KeyCopyID, "copy"))
	}
	options = append(options, named(KeyRefreshPanel, "refresh"))

	// The filter label follows the same condition as the filter BINDING, so a panel that cannot be filtered never advertises it.
	if isList && !panel.IsFilterDisabled() {
		options = append(options, named(KeyFilter, "filter"))
	}
	if isList || onMain {
		options = append(options, named(KeyActions, "actions"))
	}

	switch viewName {
	case "ecs":
		options = append(options, named(KeyECSExec, "exec"))
	case "ec2":
		options = append(options, named(KeyEC2Connect, "connect"))
	case "secrets":
		options = append(options, named(KeySecretsReveal, "reveal"))
	}

	return append(options, named(KeyQuit, "quit"))
}
