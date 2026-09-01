package ui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/config"
	"github.com/noelruault/lazyaws/ui/resources"
	"github.com/noelruault/lazyaws/ui/types"
)

type spy struct {
	mu     sync.Mutex
	ran    bool
	input  string
	action resources.Action
}

func newSpy(name string, opts ...func(*resources.Action)) *spy {
	s := &spy{action: resources.Action{Name: name}}
	for _, opt := range opts {
		opt(&s.action)
	}

	s.action.Run = func(_ context.Context, input string) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.ran, s.input = true, input
		return nil
	}

	return s
}

func (s *spy) listOfOne() []resources.Action { return []resources.Action{s.action} }

func mutating(a *resources.Action)  { a.Mutates = true }
func simple(a *resources.Action)    { a.Confirm = resources.ConfirmSimple }
func dangerous(a *resources.Action) { a.Confirm, a.Token = resources.ConfirmDangerous, "prod-db" }
func asking(a *resources.Action)    { a.Prompt = "how many?" }

// Polls because execAction deliberately runs the action off the UI thread.
func (s *spy) ranWithin(d time.Duration) bool {
	for deadline := time.Now().Add(d); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
		s.mu.Lock()
		ran := s.ran
		s.mu.Unlock()
		if ran {
			return true
		}
	}
	return false
}

func (s *spy) mustRun(t *testing.T, why string) {
	t.Helper()
	if !s.ranWithin(2 * time.Second) {
		t.Fatal(why)
	}
}

func (s *spy) mustNotRun(t *testing.T, why string) {
	t.Helper()
	if s.ranWithin(50 * time.Millisecond) {
		t.Fatal(why)
	}
}

// The condition is evaluated on the loop's goroutine, for paths that queue themselves onto the loop rather than acting inline.
func waitFor(t *testing.T, g *gocui.Gui, holds func() bool, what string) {
	t.Helper()

	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
		if ask(g, holds) {
			return
		}
	}

	t.Fatalf("timed out waiting for %s", what)
}

// The keybinding cannot be pressed on a headless screen, so the test invokes the same closure the binding holds rather than a copy of it.
func typeInPopup(t *testing.T, g *gocui.Gui, gui *Gui, text string, handler func(*gocui.Gui, *gocui.View) error) error {
	t.Helper()

	return ask(g, func() error {
		view := gui.Views.Confirmation
		view.ClearTextArea()
		view.TextArea.TypeString(text)
		view.RenderTextArea()
		return handler(g, view)
	})
}

func TestConfirmNoneRunsStraightAway(t *testing.T) {
	gui, g := newHeadlessGui(t)

	action := newSpy("Refresh thing")
	run(t, g, func() error { return gui.runAction(action.action) })

	action.mustRun(t, "an action with no confirmation did not run")
}

func TestConfirmSimpleWaitsForYes(t *testing.T) {
	gui, g := newHeadlessGui(t)

	action := newSpy("Stop thing", mutating, simple)
	run(t, g, func() error { return gui.runAction(action.action) })
	action.mustNotRun(t, "a ConfirmSimple action ran before it was confirmed")

	if !ask(g, func() bool { return gui.Views.Confirmation.Visible }) {
		t.Fatal("no confirmation popup was shown")
	}

	run(t, g, func() error { return gui.onActionConfirmed(action.action, "")(g, gui.Views.Confirmation) })
	action.mustRun(t, "confirming did not run the action")
}

func TestConfirmDangerousDemandsTheTokenTypedOut(t *testing.T) {
	gui, g := newHeadlessGui(t)

	action := newSpy("Delete thing", mutating, dangerous)
	run(t, g, func() error { return gui.runAction(action.action) })
	action.mustNotRun(t, "a dangerous action ran before anything was typed")

	// Typos re-prompt so users do not learn to paste confirmation tokens.
	if err := typeInPopup(t, g, gui, "prod-d", gui.onDangerousToken(action.action, "")); err != nil {
		t.Fatalf("typing a wrong token: %v", err)
	}
	action.mustNotRun(t, "a dangerous action ran on the wrong token")

	if err := typeInPopup(t, g, gui, "prod-db", gui.onDangerousToken(action.action, "")); err != nil {
		t.Fatalf("typing the token: %v", err)
	}
	action.mustRun(t, "the right token did not run the action")
}

func TestPromptedInputReachesRun(t *testing.T) {
	gui, g := newHeadlessGui(t)

	action := newSpy("Resize thing", mutating, asking)
	run(t, g, func() error { return gui.runAction(action.action) })
	action.mustNotRun(t, "a prompting action ran before it was answered")

	if err := typeInPopup(t, g, gui, "t3.large", gui.onActionInput(action.action)); err != nil {
		t.Fatalf("answering the prompt: %v", err)
	}
	action.mustRun(t, "answering the prompt did not run the action")

	action.mu.Lock()
	defer action.mu.Unlock()
	if action.input != "t3.large" {
		t.Fatalf("Run received %q, want what was typed", action.input)
	}
}

func TestPromptThenConfirmKeepsTheInput(t *testing.T) {
	gui, g := newHeadlessGui(t)

	action := newSpy("Upgrade thing", mutating, asking, simple)
	run(t, g, func() error { return gui.runAction(action.action) })

	if err := typeInPopup(t, g, gui, "1.30", gui.onActionInput(action.action)); err != nil {
		t.Fatalf("answering the prompt: %v", err)
	}
	action.mustNotRun(t, "the value was accepted as the confirmation")

	run(t, g, func() error { return gui.onActionConfirmed(action.action, "1.30")(g, gui.Views.Confirmation) })
	action.mustRun(t, "confirming did not run the action")

	action.mu.Lock()
	defer action.mu.Unlock()
	if action.input != "1.30" {
		t.Fatalf("Run received %q, want the value entered before the confirmation", action.input)
	}
}

// The persistent text area must be cleared between two-step prompts.
func TestPromptThenTokenStartsTheSecondPromptEmpty(t *testing.T) {
	gui, g := newHeadlessGui(t)

	action := newSpy("Delete thing", mutating, asking, dangerous)
	run(t, g, func() error { return gui.runAction(action.action) })

	if err := typeInPopup(t, g, gui, "7", gui.onActionInput(action.action)); err != nil {
		t.Fatalf("answering the prompt: %v", err)
	}

	if leftover := ask(g, func() string { return gui.trimmedContent(gui.Views.Confirmation) }); leftover != "" {
		t.Fatalf("the token prompt opened holding %q from the previous prompt", leftover)
	}

	if err := typeInPopup(t, g, gui, "prod-db", gui.onDangerousToken(action.action, "7")); err != nil {
		t.Fatalf("typing the token: %v", err)
	}
	action.mustRun(t, "the exact token did not satisfy the gate")

	action.mu.Lock()
	defer action.mu.Unlock()
	if action.input != "7" {
		t.Fatalf("Run received %q, want the value entered before the token", action.input)
	}
}

// Runtime-built dangerous actions must reject an empty token before execution.
func TestAMalformedActionNeverReachesAWS(t *testing.T) {
	gui, g := newHeadlessGui(t)

	tokenless := newSpy("Delete thing", mutating)
	tokenless.action.Confirm = resources.ConfirmDangerous

	run(t, g, func() error { return gui.runAction(tokenless.action) })
	tokenless.mustNotRun(t, "a dangerous action with no token was allowed to run")
}

// Execution must enforce read-only mode even when callers bypass menu filtering.
func TestReadOnlyRefusesEvenWhenTheMenuIsBypassed(t *testing.T) {
	user := config.DefaultUserConfig()
	user.ReadOnly = true
	gui, g := newHeadlessGuiWithConfig(t, user)

	action := newSpy("Terminate thing", mutating)
	run(t, g, func() error { return gui.runAction(action.action) })
	action.mustNotRun(t, "read-only mode ran a mutating action")

	safe := newSpy("Preview thing")
	run(t, g, func() error { return gui.runAction(safe.action) })
	safe.mustRun(t, "read-only mode blocked an action that changes nothing")
}

func TestActionsKeyAsksTheRegistry(t *testing.T) {
	gui, g := newHeadlessGui(t)

	gui.Panels.ECR.SetItems([]*aws.ECRRepository{{Name: "svc-api"}})
	run(t, g, func() error {
		_, err := g.SetCurrentView("ecr")
		return err
	})

	run(t, g, gui.handleActionsMenu)

	labels := menuLabels(g, gui)
	if len(labels) == 0 {
		t.Fatal("the actions key opened nothing on the ECR panel")
	}
	if !hasLabelContaining(labels, "Delete repository") {
		t.Errorf("ECR actions = %v, want the repository delete among them", labels)
	}
}

// An empty list has no selection, so there is nothing an action could act on and the key must stay silent rather than open a box with no rows.
func TestActionsKeyDoesNothingWhereThereIsNothingToDo(t *testing.T) {
	gui, g := newHeadlessGui(t)

	gui.Panels.Profile.SetItems(nil)
	run(t, g, func() error {
		_, err := g.SetCurrentView("profile")
		return err
	})

	run(t, g, gui.handleActionsMenu)

	if ask(g, func() bool { return gui.Views.Menu.Visible }) {
		t.Error("the actions key opened an empty menu")
	}
}

// A connected profile that cannot reach AWS is the one case where the connected profile has an action: the login that gets it working again.
// Without it the Credentials tab tells the user to press a and the menu opens on nothing.
func TestTheConnectedProfileOffersLoginWhileItsCredentialsAreBroken(t *testing.T) {
	gui, g := newHeadlessGui(t)

	gui.CurrentProfile = "prod"
	gui.Panels.Profile.SetItems([]string{"prod"})

	var labels []string
	for _, action := range gui.ProfileActions() {
		labels = append(labels, action.Name)
	}
	if !hasLabelContaining(labels, "aws sso login --profile prod") {
		t.Errorf("profile actions = %v, want the login command among them", labels)
	}

	run(t, g, func() error {
		_, err := g.SetCurrentView("profile")
		return err
	})
	run(t, g, gui.handleActionsMenu)

	if !ask(g, func() bool { return gui.Views.Menu.Visible }) {
		t.Error("the actions key opened nothing on a profile that cannot reach AWS")
	}
}

// Every dangerous action must require a non-empty token.
func TestEveryDangerousActionHasAToken(t *testing.T) {
	for name, actions := range shippedActions(t) {
		for _, action := range actions {
			if err := action.Valid(); err != nil {
				t.Errorf("%s / %q: %v", name, action.Name, err)
			}
		}
	}
}

// Destructive actions must not survive read-only menu filtering.
func TestDestructiveActionsAreMarkedMutating(t *testing.T) {
	// These verbs identify mutations without misclassifying Preview or Switch.
	changesThings := []string{"delete", "terminate", "remove", "abort", "rotate", "upgrade", "stop", "start", "reboot", "enable", "disable", "edit", "set ", "create", "change", "exec"}

	for name, actions := range shippedActions(t) {
		for _, action := range actions {
			if action.Mutates {
				continue
			}
			lower := strings.ToLower(action.Name)
			for _, verb := range changesThings {
				if strings.HasPrefix(lower, verb) {
					t.Errorf("%s / %q reads like it changes AWS but is not marked Mutates, so read-only mode would run it", name, action.Name)
				}
			}
		}
	}
}

// Confirmation tokens must be visible identifiers users can retype.
func TestTokensAreWhatYouCanRead(t *testing.T) {
	for name, actions := range shippedActions(t) {
		for _, action := range actions {
			if action.Confirm != resources.ConfirmDangerous {
				continue
			}
			if strings.HasPrefix(action.Token, "arn:") {
				t.Errorf("%s / %q asks for an ARN to be typed out", name, action.Name)
			}
			if len(action.Token) > 60 {
				t.Errorf("%s / %q asks for a %d-character token", name, action.Name, len(action.Token))
			}
		}
	}
}

// Real action lists keep new actions under these invariants automatically.
func shippedActions(t *testing.T) map[string][]resources.Action {
	t.Helper()

	gui, _ := newHeadlessGui(t)

	gui.CurrentProfile = "default"
	gui.Panels.Profile.SetItems([]string{"staging"})
	gui.Panels.EC2.SetItems([]*aws.Instance{{ID: "i-1", Name: "web"}})
	gui.Panels.S3.SetItems([]*aws.Bucket{{Name: "bucket"}})
	gui.Panels.EKS.SetItems([]*aws.EKSCluster{{Name: "cluster", Version: "1.29"}})
	gui.Panels.ECR.SetItems([]*aws.ECRRepository{{Name: "repo"}})
	gui.Panels.Secrets.SetItems([]*aws.SecretSummary{{Name: "secret", HasReplication: true}})

	lists := map[string][]resources.Action{
		"profiles": gui.ProfileActions(),
		"ec2":      gui.EC2Actions(),
		"s3":       gui.S3Actions(),
		"eks":      gui.EKSActions(),
		"ecr":      gui.ECRActions(),
		"secrets":  gui.SecretsActions(),
	}

	deleted := time.Now()
	gui.Panels.Secrets.SetItems([]*aws.SecretSummary{{Name: "secret", DeletedDate: &deleted}})
	lists["secrets (pending deletion)"] = gui.SecretsActions()

	for name, level := range map[string]struct {
		drill ecsDrillState
		row   *ecsRow
	}{
		"ecs clusters": {ecsDrillState{level: ecsLevelClusters}, &ecsRow{Kind: ecsRowKindCluster, Cluster: &aws.ECSCluster{Name: "prod"}}},
		"ecs services": {ecsDrillState{level: ecsLevelServices, cluster: "prod"}, &ecsRow{Kind: ecsRowKindService, Service: &aws.ECSService{Name: "api", DesiredCount: 2}}},
		"ecs tasks":    {ecsDrillState{level: ecsLevelTasks, cluster: "prod", service: "api"}, &ecsRow{Kind: ecsRowKindTask, Task: &aws.ECSTask{ID: "abc123", Arn: "arn:task/abc123", Containers: []aws.ECSContainer{{Name: "web"}}}}},
	} {
		lists[name] = ecsAt(t, gui, level.drill, level.row)
	}

	for name, actions := range lists {
		if len(actions) == 0 {
			t.Errorf("%s offers no actions at all, which is probably not what was meant", name)
		}
	}

	return lists
}

func menuLabels(g *gocui.Gui, gui *Gui) []string {
	var labels []string
	for _, item := range ask(g, func() []*types.MenuItem { return gui.Panels.Menu.List.GetItems() }) {
		label := item.Label
		if label == "" && len(item.LabelColumns) > 0 {
			label = item.LabelColumns[0]
		}
		if label != "cancel" {
			labels = append(labels, label)
		}
	}
	return labels
}

func hasLabelContaining(labels []string, needle string) bool {
	for _, label := range labels {
		if strings.Contains(label, needle) {
			return true
		}
	}
	return false
}
