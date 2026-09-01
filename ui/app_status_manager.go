package ui

import (
	"sync"
	"time"

	"github.com/noelruault/lazyaws/ui/presentation"
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
	return presentation.VersionBullet(gui.Version, gui.updateState)
}

// renderGlobalOptions draws the dashboard footer, which is one entry: the key that opens the menu listing every binding for the focused view.
// The menu is contextual and complete, so printing a subset of the same keys along the bottom row spent a whole terminal row restating what one keycap already reaches.
func (gui *Gui) renderGlobalOptions() error {
	help := option{key: describeKey(gui.Keys.Get(KeyHelp).Key), label: "keys"}

	return gui.renderString(gui.g, "options", optionsToString([]option{help}))
}
