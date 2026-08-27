// Package ui is the lazyaws port of lazydocker's gocui TUI (MIT, © 2018 Jesse Duffield).
package ui

import (
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/config"
	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/resources"
	"github.com/noelruault/lazyaws/ui/tasks"
	"github.com/noelruault/lazyaws/ui/types"
)

type Gui struct {
	g           *gocui.Gui
	Log         *slog.Logger
	State       guiState
	Config      *config.Config
	taskManager *tasks.TaskManager
	ErrorChan   chan error
	Views       Views

	// Registry drives both navigation and actions so resource names cannot drift.
	Registry *resources.Registry

	Keys Keymap

	// startupProblems keeps invalid key overrides non-fatal.
	startupProblems []error

	dimmed map[string]gocui.Attribute

	statusManager *statusManager

	Version string

	// PauseBackgroundThreads protects subprocess ownership of the tty.
	// Atomic because the refresh tiers read it from their own goroutines while the subprocess helper sets it from the UI loop.
	PauseBackgroundThreads atomic.Bool

	Mutexes

	Panels Panels

	// Client is replaced wholesale so every service shares profile credentials.
	Client *aws.Client

	CurrentProfile string

	// Gen prevents superseded profile results from reaching the UI.
	Gen int

	Profiles []string

	// authProblem stays non-fatal so profile switching remains a recovery path.
	authProblem error

	ecsDrill ecsDrillState

	s3Objects s3ObjectsState

	secretsReveal secretsRevealState

	secretsShowDeleted bool

	// mainCursorState is the selection inside the main panel; only one navigable list shows at a time.
	mainCursorState mainCursorState

	vpcEndpoints vpcEndpointsState

	// throttledRefresh collapses bursts into one AWS reload per 50ms window.
	throttledRefresh *throttle

	// panelThrottles bound goroutine creation from repeated refresh keys.
	panelThrottles map[string]*throttle

	// panelReloads holds each panel's loader behind its single-flight guard, keyed as panelReloaders keys them.
	panelReloads map[string]func() error

	// throttles carries a throttled fetch from the overview that saw it to the pane gate that has to slow down because of it.
	throttles throttleWatch

	// mainWidth is main's inner width as of the last layout pass, so a resize can be told from the many layout passes that change nothing.
	mainWidth int

	ec2Extras ec2OverviewExtras

	// rerenderMainTab collapses a drag-resize into one re-render per 50ms window.
	rerenderMainTab *throttle
}

type Panels struct {
	Profile *panels.SideListPanel[string]
	ECS     *panels.SideListPanel[*ecsRow]
	EC2     *panels.SideListPanel[*aws.Instance]
	S3      *panels.SideListPanel[*aws.Bucket]
	EKS     *panels.SideListPanel[*aws.EKSCluster]
	ECR     *panels.SideListPanel[*aws.ECRRepository]
	Secrets *panels.SideListPanel[*aws.SecretSummary]

	VPC *panels.SideListPanel[*aws.VPC]

	Menu *panels.SideListPanel[*types.MenuItem]
}

type Mutexes struct {
	SubprocessMutex sync.Mutex
	ViewStackMutex  sync.Mutex
}

// mainPanelState uses ObjectKey to avoid restarting an unchanged selection's task.
type mainPanelState struct {
	ObjectKey string
}

type panelStates struct {
	Main *mainPanelState
}

type guiState struct {
	ViewStack  []string
	Panels     *panelStates
	ScreenMode WindowMaximisation

	Filter filterState

	Command commandState

	// Settings and Q remain pointers so guiState stays copyable despite their mutexes.
	Settings *settingsState
	Q        *qState
}

type filterState struct {
	active bool
	panel  panels.ISideListPanel
	needle string
}

type WindowMaximisation int

const (
	SCREEN_NORMAL WindowMaximisation = iota
	SCREEN_HALF
	SCREEN_FULL
)

func getScreenMode(cfg *config.Config) WindowMaximisation {
	switch cfg.User.Gui.ScreenMode {
	case "half":
		return SCREEN_HALF
	case "fullscreen":
		return SCREEN_FULL
	default:
		return SCREEN_NORMAL
	}
}

func NewGui(cfg *config.Config, client *aws.Client, errorChan chan error) (*Gui, error) {
	initialState := guiState{
		Panels: &panelStates{
			Main: &mainPanelState{ObjectKey: ""},
		},
		ViewStack:  []string{},
		ScreenMode: getScreenMode(cfg),
		Q:          &qState{},
		Settings:   &settingsState{},
	}

	gui := &Gui{
		Log:           slog.Default(),
		State:         initialState,
		Config:        cfg,
		statusManager: &statusManager{},
		taskManager:   tasks.NewTaskManager(slog.Default()),
		ErrorChan:     errorChan,
		Client:        client,
	}
	gui.Registry = gui.newRegistry()
	gui.Keys, gui.startupProblems = buildKeymap(cfg.User.Keybindings)
	gui.throttledRefresh = newThrottle(50*time.Millisecond, gui.refresh)
	gui.rerenderMainTab = newThrottle(50*time.Millisecond, gui.rerenderCurrentMainTab)
	// Wrapped once, here, so every path into a loader shares one in-flight guard: a throttle firing, a refresh key and a tick from the panel tier are three callers that must not turn into three concurrent reloads of the same list.
	gui.panelReloads = make(map[string]func() error)
	for name, reload := range gui.panelReloaders() {
		gui.panelReloads[name] = singleFlight(reload)
	}
	gui.panelThrottles = make(map[string]*throttle)
	for name, reload := range gui.panelReloads {
		gui.panelThrottles[name] = newThrottle(50*time.Millisecond, func() { go func() { _ = reload() }() })
	}

	return gui, nil
}

// panelReloaders keeps full and focused refresh paths aligned.
func (gui *Gui) panelReloaders() map[string]func() error {
	return map[string]func() error{
		profileReloader: gui.refreshProfile,
		"ecs":           gui.loadECSList,
		"ec2":           gui.loadEC2List,
		"s3":            gui.loadS3List,
		"eks":           gui.loadEKSList,
		"ecr":           gui.loadECRList,
		"secrets":       gui.loadSecretsList,
		"vpc":           gui.loadVPCList,
	}
}

// refresh runs loaders concurrently because each rejects stale profile results.
// It goes through the single-flighted loaders rather than panelReloaders, so a full refresh landing on top of a panel tier's tick reloads each list once instead of twice.
func (gui *Gui) refresh() {
	// The profile panel needs no AWS credentials and must remain available as the recovery path.
	if gui.authProblem != nil || !gui.Client.Ready() {
		go func() { _ = gui.reloadProfilePanel() }()
		gui.showAuthProblem()
		return
	}

	for _, reload := range gui.panelReloads {
		go func() { _ = reload() }()
	}
}

// reloadProfilePanel reloads the profile list through its single-flight guard, falling back to the loader itself if the guard was never built (a Gui assembled outside NewGui).
func (gui *Gui) reloadProfilePanel() error {
	if reload, ok := gui.panelReloads[profileReloader]; ok {
		return reload()
	}

	return gui.refreshProfile()
}

func (gui *Gui) showAuthProblem() {
	if gui.authProblem == nil {
		return
	}

	gui.State.Panels.Main.ObjectKey = "auth-problem"
	gui.reRenderStringMain(degradedModeMessage(gui.CurrentProfile, gui.authProblem))
}

// ShouldRefresh records the key so unchanged selections reuse their render task.
func (gui *Gui) ShouldRefresh(key string) bool {
	if gui.State.Panels.Main.ObjectKey == key {
		return false
	}

	gui.State.Panels.Main.ObjectKey = key
	return true
}

func (gui *Gui) IgnoreStrings() []string {
	return nil
}

func (gui *Gui) Update(f func() error) {
	gui.g.Update(func(*gocui.Gui) error { return f() })
}

func (gui *Gui) goEvery(interval time.Duration, function func() error) {
	_ = function() // time.Tick doesn't run immediately, so fire once up front
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if !gui.PauseBackgroundThreads.Load() {
				_ = function()
			}
		}
	}()
}

func (gui *Gui) Run() error {
	defer gui.taskManager.Close()

	// Before gocui takes the terminal, so the sequence lands on the main screen it is clearing.
	blockScrollback(os.Stdout)

	g, err := gocui.NewGui(gocui.NewGuiOpts{
		OutputMode:       gocui.OutputTrue,
		RuneReplacements: map[rune]string{},
	})
	if err != nil {
		return err
	}
	defer g.Close()

	g.Mouse = !gui.Config.User.Gui.IgnoreMouseEvents

	gui.g = g

	if err := gui.SetColorScheme(); err != nil {
		return err
	}

	go func() {
		for err := range gui.ErrorChan {
			if err == nil {
				continue
			}
			_ = gui.createErrorPanel(err.Error())
		}
	}()

	g.SetManager(gocui.ManagerFunc(gui.layout), gocui.ManagerFunc(gui.getFocusLayout()))

	if err := gui.createAllViews(); err != nil {
		return err
	}

	gui.setPanels()

	if err := gui.keybindings(g); err != nil {
		return err
	}

	if gui.g.CurrentView() == nil {
		view, err := gui.g.View(gui.initiallyFocusedViewName())
		if err != nil {
			return err
		}
		if err := gui.switchFocus(view); err != nil {
			return err
		}
	}

	gui.reportStartupProblems()

	gui.throttledRefresh.Trigger()
	gui.startAutoRefresh()

	err = g.MainLoop()
	if err == gocui.ErrQuit {
		return nil
	}
	return err
}

func (gui *Gui) setPanels() {
	gui.Panels = Panels{
		Profile: gui.getProfilePanel(),
		ECS:     gui.getECSPanel(),
		EC2:     gui.getEC2Panel(),
		S3:      gui.getS3Panel(),
		EKS:     gui.getEKSPanel(),
		ECR:     gui.getECRPanel(),
		Secrets: gui.getSecretsPanel(),

		VPC: gui.getVPCPanel(),

		Menu: gui.getMenuPanel(),
	}
}
