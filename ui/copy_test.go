package ui

import (
	"strings"
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/panels"
)

// Every fixture's name differs from its ARN, because a panel reading the wrong field passes against a resource whose name IS its identifier.
// The panels come from the real constructors rather than from hand-built literals: what is under test is which field each panel publishes, and a test that wires its own CopyValue asserts only itself.
func TestEveryPanelCopiesItsFullIdentifier(t *testing.T) {
	cases := []struct {
		view string
		want string
		seed func(*Gui) panels.ISideListPanel
	}{
		{view: "profile", want: "staging", seed: func(gui *Gui) panels.ISideListPanel {
			gui.Panels.Profile.SetItems([]string{"staging"})
			return gui.Panels.Profile
		}},
		{view: "ecs", want: "arn:aws:ecs:eu-west-1:111122223333:service/prod/api", seed: func(gui *Gui) panels.ISideListPanel {
			gui.Panels.ECS.SetItems([]*ecsRow{{
				Kind:    ecsRowKindService,
				Service: &aws.ECSService{Name: "api", Arn: "arn:aws:ecs:eu-west-1:111122223333:service/prod/api"},
			}})
			return gui.Panels.ECS
		}},
		{view: "ec2", want: "i-0abcdef1234567890", seed: func(gui *Gui) panels.ISideListPanel {
			gui.Panels.EC2.SetItems([]*aws.Instance{{ID: "i-0abcdef1234567890", Name: "web-1"}})
			return gui.Panels.EC2
		}},
		{view: "s3", want: "acme-prod-logs", seed: func(gui *Gui) panels.ISideListPanel {
			gui.Panels.S3.SetItems([]*aws.Bucket{{Name: "acme-prod-logs"}})
			return gui.Panels.S3
		}},
		{view: "eks", want: "arn:aws:eks:eu-west-1:111122223333:cluster/prod", seed: func(gui *Gui) panels.ISideListPanel {
			gui.Panels.EKS.SetItems([]*aws.EKSCluster{{Name: "prod", Arn: "arn:aws:eks:eu-west-1:111122223333:cluster/prod"}})
			return gui.Panels.EKS
		}},
		{view: "ecr", want: "arn:aws:ecr:eu-west-1:111122223333:repository/api", seed: func(gui *Gui) panels.ISideListPanel {
			gui.Panels.ECR.SetItems([]*aws.ECRRepository{{Name: "api", Arn: "arn:aws:ecr:eu-west-1:111122223333:repository/api"}})
			return gui.Panels.ECR
		}},
		{view: "secrets", want: "arn:aws:secretsmanager:eu-west-1:111122223333:secret:prod/db/password-AbCdEf", seed: func(gui *Gui) panels.ISideListPanel {
			gui.Panels.Secrets.SetItems([]*aws.SecretSummary{{Name: "prod/db/password", Arn: "arn:aws:secretsmanager:eu-west-1:111122223333:secret:prod/db/password-AbCdEf"}})
			return gui.Panels.Secrets
		}},
		{view: "vpc", want: "vpc-0abcdef1234567890", seed: func(gui *Gui) panels.ISideListPanel {
			gui.Panels.VPC.SetItems([]*aws.VPC{{ID: "vpc-0abcdef1234567890", Name: "main"}})
			return gui.Panels.VPC
		}},
	}

	// A ninth panel with no entry here would ship with no copy value and nothing failing.
	if got, want := len(cases), len(newTestGui(t).allSidePanels()); got != want {
		t.Fatalf("%d panels covered, %d exist: every list has a copy value or the key does nothing on it", got, want)
	}

	for _, tc := range cases {
		t.Run(tc.view, func(t *testing.T) {
			gui := newTestGui(t)

			got, ok := tc.seed(gui).SelectedCopyValue()
			if !ok {
				t.Fatalf("%s reports nothing to copy for a seeded row", tc.view)
			}
			if got != tc.want {
				t.Errorf("%s copies %q, want %q", tc.view, got, tc.want)
			}
		})
	}
}

// The three panels that carry an ARN carry it from a list call that can answer without one, and a blank popup is worse than the name.
func TestCopyValueFallsBackToTheNameWithoutAnArn(t *testing.T) {
	gui := newTestGui(t)

	gui.Panels.ECR.SetItems([]*aws.ECRRepository{{Name: "api"}})
	if got, ok := gui.Panels.ECR.SelectedCopyValue(); !ok || got != "api" {
		t.Errorf("ECR with no ARN copies (%q, %v), want (\"api\", true)", got, ok)
	}

	gui.Panels.Secrets.SetItems([]*aws.SecretSummary{{Name: "prod/db/password"}})
	if got, ok := gui.Panels.Secrets.SelectedCopyValue(); !ok || got != "prod/db/password" {
		t.Errorf("a secret with no ARN copies (%q, %v), want (\"prod/db/password\", true)", got, ok)
	}

	gui.Panels.EKS.SetItems([]*aws.EKSCluster{{Name: "prod"}})
	if got, ok := gui.Panels.EKS.SelectedCopyValue(); !ok || got != "prod" {
		t.Errorf("an EKS cluster with no ARN copies (%q, %v), want (\"prod\", true)", got, ok)
	}
}

// A row identifying itself with nothing is the one case that must not open a popup: an empty confirmation panel reads as a bug in the app, not as an absent field.
func TestCopyValueReportsNothingToCopy(t *testing.T) {
	gui := newTestGui(t)

	if got, ok := gui.Panels.EC2.SelectedCopyValue(); ok {
		t.Errorf("an empty list copies (%q, true), want nothing to copy", got)
	}

	gui.Panels.ECR.SetItems([]*aws.ECRRepository{{}})
	if got, ok := gui.Panels.ECR.SelectedCopyValue(); ok {
		t.Errorf("a row with neither ARN nor name copies (%q, true), want nothing to copy", got)
	}
}

// The key is registered per view rather than globally: the menu and confirmation popups bind y as "yes", so a global copy binding would be shadowed on the views that already answer to it.
func TestCopyKeyIsBoundOnEveryListAndOnMain(t *testing.T) {
	gui, _ := newHeadlessGui(t)

	bound := map[string]bool{}
	for _, binding := range gui.GetInitialKeybindings() {
		if binding.Name == KeyCopyID {
			bound[binding.ViewName] = true
		}
	}

	if bound[""] {
		t.Error("copy-id is bound globally, so it would fire in the chat input and the filter prompt")
	}

	want := resourceViewNames(gui.allSidePanels())
	for _, name := range want {
		if !bound[name] {
			t.Errorf("copy-id is not bound in the %q view", name)
		}
	}
	if len(bound) != len(want) {
		t.Errorf("copy-id is bound in %d views, want the %d that address a selected resource", len(bound), len(want))
	}
}

// Headless, because the handler resolves its panel through the focused view's NAME: with views built as literals the lookup answers "profile" for every stack and the assertion lands on the wrong panel.
// The stack is a list under main, which is focus having moved into the detail pane, and the pane still describes the list's selected row.
func TestCopyShowsTheFullIdentifierInAPopup(t *testing.T) {
	gui, g := newHeadlessGui(t)

	const arn = "arn:aws:secretsmanager:eu-west-1:111122223333:secret:prod/db/password-AbCdEf"
	run(t, g, func() error {
		gui.Panels.Secrets.SetItems([]*aws.SecretSummary{{Name: "prod/db/password", Arn: arn}})
		return nil
	})
	gui.State.ViewStack = []string{"secrets", "main"}

	run(t, g, gui.handleCopySelected)

	body := waitForView(t, g, gui.Views.Confirmation, "arn:aws:secretsmanager")
	// The popup wraps a value this long over several rows, so the assertion is on the value with every wrap point removed: an ARN has no whitespace of its own, and equality then still fails on anything cut or elided.
	if got := strings.Join(strings.Fields(body), ""); got != arn {
		t.Errorf("the popup shows %q, want the untruncated %q", got, arn)
	}
	if title := ask(g, func() string { return gui.Views.Confirmation.Title }); title != copyPopupTitle {
		t.Errorf("popup title = %q, want %q", title, copyPopupTitle)
	}
}

func TestCopyOpensNoPopupWithNothingSelected(t *testing.T) {
	gui, g := newHeadlessGui(t)
	gui.State.ViewStack = []string{"ec2"}

	// Proof the panel lookup itself succeeds, so a miss cannot stand in for the guard under test.
	if _, ok := gui.sidePanelForMain(); !ok {
		t.Fatal("the focused view resolves to no panel in this harness, so this test could not observe the guard")
	}

	run(t, g, gui.handleCopySelected)

	if visible := ask(g, func() bool { return gui.Views.Confirmation.Visible }); visible {
		t.Error("an empty list opened a copy popup, want nothing to copy")
	}
}
