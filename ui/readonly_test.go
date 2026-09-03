package ui

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/noelruault/lazyaws/config"
	"github.com/noelruault/lazyaws/ui/resources"
	"github.com/noelruault/lazyaws/ui/types"
	"github.com/noelruault/lazyaws/ui/utils"
)

// The default has to be safe without anyone having read anything, so this asserts the state of a plain `lazyaws` run rather than the state of a config file.
func TestWithoutTheFlagTheSessionIsReadOnly(t *testing.T) {
	gui, _ := newReadOnlyHeadlessGui(t)

	if !gui.readOnly() {
		t.Error("a session started without --allow-writes is not read-only")
	}

	// A nil config is the case a future caller reaches by forgetting to pass one, and it must not be the case that grants writes.
	if !(&Gui{}).readOnly() {
		t.Error("a Gui with no config at all reports that writes are allowed")
	}
}

func TestTheFlagIsWhatMakesTheSessionWritable(t *testing.T) {
	gui, _ := newHeadlessGuiWithAppConfig(t, config.Config{User: config.DefaultUserConfig(), AllowWrites: true})

	if gui.readOnly() {
		t.Error("--allow-writes did not make the session writable")
	}
}

// The config file may tighten the flag and must never loosen it: a readOnly: true inherited from a dotfile repo has to win, and a readOnly: false must not grant what the command line withheld.
func TestTheConfigFileCanOnlyTightenThePolicy(t *testing.T) {
	tightened := config.DefaultUserConfig()
	tightened.ReadOnly = true
	gui, _ := newHeadlessGuiWithAppConfig(t, config.Config{User: tightened, AllowWrites: true})
	if !gui.readOnly() {
		t.Error("readOnly in the config file did not survive --allow-writes")
	}

	loosened := config.DefaultUserConfig()
	loosened.ReadOnly = false
	gui, _ = newHeadlessGuiWithAppConfig(t, config.Config{User: loosened})
	if !gui.readOnly() {
		t.Error("readOnly: false in the config file granted writes that the command line withheld")
	}
}

// runAction is where a keypress becomes an AWS call, so a mutating action reaching it in a read-only session must stop there.
func TestAMutatingActionIsRefusedWithoutTheFlag(t *testing.T) {
	gui, g := newReadOnlyHeadlessGui(t)

	// A channel rather than a bool: the action body runs on its own goroutine, so a plain variable would be a data race in exactly the case this test is about.
	ran := make(chan struct{}, 1)
	action := resources.Action{
		Name:    "Terminate instance",
		Mutates: true,
		Run:     func(context.Context, string) error { ran <- struct{}{}; return nil },
	}

	run(t, g, func() error { return gui.runAction(action) })

	select {
	case <-ran:
		t.Fatal("the action ran in a read-only session")
	case <-time.After(250 * time.Millisecond):
	}

	panel := waitForView(t, g, gui.Views.Confirmation, "read-only")
	if !strings.Contains(panel, "--allow-writes") {
		t.Errorf("the refusal does not name the flag that lifts it:\n%s", panel)
	}
}

// A non-mutating action must still work, or read-only would mean nothing works.
func TestANonMutatingActionStillRunsWithoutTheFlag(t *testing.T) {
	gui, g := newReadOnlyHeadlessGui(t)

	ran := make(chan struct{}, 1)
	action := resources.Action{
		Name: "Copy the ARN",
		Run:  func(context.Context, string) error { ran <- struct{}{}; return nil },
	}

	// An action with no Confirm fires straight through execAction, so the run itself is the assertion: it either happened or the gate is refusing reads too.
	run(t, g, func() error { return gui.runAction(action) })

	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("a read-only-safe action never ran in a read-only session")
	}
}

func TestMutatingMenuItemsAreHiddenWithoutTheFlag(t *testing.T) {
	gui, _ := newReadOnlyHeadlessGui(t)

	items := []*types.MenuItem{
		{Label: "Show the full ARN"},
		{Label: "Stop the task", Mutates: true},
		{Label: "Scale the service", Mutates: true},
	}

	kept := gui.dropMutatingItems(items)
	if len(kept) != 1 || kept[0].Label != "Show the full ARN" {
		labels := make([]string, 0, len(kept))
		for _, item := range kept {
			labels = append(labels, item.Label)
		}
		t.Errorf("read-only menu kept %v, want only the read-only-safe item", labels)
	}
}

// The shells lazyaws opens are `aws ecs execute-command` and `aws ssm start-session`, child processes with their own credentials, so the SDK guard cannot see them and this gate is the only thing standing there.
func TestRunSubprocessIsRefusedWithoutTheFlag(t *testing.T) {
	gui, g := newReadOnlyHeadlessGui(t)

	// A command that would be obvious if it ever ran: it writes a file nothing else creates.
	marker := t.TempDir() + "/it-ran"
	cmd := exec.Command("touch", marker)

	run(t, g, func() error { return gui.runSubprocess(cmd) })

	if _, err := exec.Command("test", "-e", marker).Output(); err == nil {
		t.Fatal("runSubprocess executed the child in a read-only session")
	}

	panel := waitForView(t, g, gui.Views.Confirmation, "read-only")
	if !strings.Contains(panel, "--allow-writes") {
		t.Errorf("the subprocess refusal does not name the flag:\n%s", panel)
	}
}

// Toggling the Settings row off cannot grant writes, because the guard reads the flag, so the row has to say so instead of appearing to work.
func TestTheReadOnlySettingCannotGrantWrites(t *testing.T) {
	gui, g := newReadOnlyHeadlessGui(t)

	index := -1
	for i, row := range gui.settings() {
		if row.name == readOnlySettingName {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatalf("no %q row in Settings", readOnlySettingName)
	}
	gui.State.Settings.selected = index

	run(t, g, func() error { return gui.handleSettingsToggle() })

	if !gui.readOnly() {
		t.Fatal("toggling the Settings row granted writes without the flag")
	}
	panel := waitForView(t, g, gui.Views.Confirmation, "allow-writes")
	if !strings.Contains(panel, "cannot be given write access") {
		t.Errorf("the Settings refusal does not explain itself:\n%s", panel)
	}
}

// A promise nobody can see is not reassurance, so the footer leads with the mode and never omits it.
func TestTheFooterLeadsWithTheModeBadge(t *testing.T) {
	gui, g := newReadOnlyHeadlessGui(t)

	run(t, g, gui.renderGlobalOptions)
	footer := utils.Decolorise(waitForView(t, g, gui.Views.Options, readOnlyBadge))

	badge := strings.Index(footer, "["+readOnlyBadge+"]")
	keys := strings.Index(footer, "Keys")
	switch {
	case badge < 0:
		t.Errorf("footer = %q, want it to carry [%s]", footer, readOnlyBadge)
	case keys < 0:
		t.Errorf("footer = %q, want it to keep the keys hint", footer)
	case badge > keys:
		t.Errorf("footer = %q, want the badge before the keys hint", footer)
	}
	if strings.Contains(footer, writesBadge) {
		t.Errorf("footer = %q, want no writes badge in a read-only session", footer)
	}

	// The writable session is the one worth noticing, so it says so rather than saying nothing.
	writable, wg := newHeadlessGuiWithAppConfig(t, config.Config{User: config.DefaultUserConfig(), AllowWrites: true})
	run(t, wg, writable.renderGlobalOptions)
	line := utils.Decolorise(waitForView(t, wg, writable.Views.Options, writesBadge))
	if !strings.HasPrefix(strings.TrimSpace(line), "["+writesBadge+"]") {
		t.Errorf("footer = %q, want it to open with [%s] once writes are allowed", line, writesBadge)
	}
	if strings.Contains(line, readOnlyBadge) {
		t.Errorf("footer = %q, want no read-only badge when writes are allowed", line)
	}
}
