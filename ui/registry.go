// Package ui keeps provider-aware references behind one gocui translation seam.
package ui

import (
	"fmt"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/ui/panels"
	awsprovider "github.com/noelruault/lazyaws/ui/providers/aws"
	"github.com/noelruault/lazyaws/ui/resources"
)

// newRegistry registers eagerly so duplicate names panic during construction instead of on first use.
func (gui *Gui) newRegistry() *resources.Registry {
	registry := resources.NewRegistry(awsprovider.Provider)
	awsprovider.Register(registry, gui)
	return registry
}

// panelRefs bridges gocui view names that the provider registry cannot know; exhaustive tests prevent drift.
func panelRefs() map[string]resources.Key {
	ref := func(service, resource string) resources.Key {
		return resources.Key{Provider: awsprovider.Provider, Service: service, Resource: resource}
	}

	return map[string]resources.Key{
		"profile": ref("profiles", ""),
		"ecs":     ref("ecs", "clusters"),
		"ec2":     ref("ec2", "instances"),
		"s3":      ref("s3", "buckets"),
		"eks":     ref("eks", "clusters"),
		"ecr":     ref("ecr", "repositories"),
		"secrets": ref("secretsmanager", "secrets"),
		"vpc":     ref("vpc", "vpcs"),
	}
}

func (gui *Gui) goToPanel(view *gocui.View) error {
	gui.dismissPopups()
	gui.leaveQScreen()
	gui.State.Settings.active = false
	gui.resetMainView()
	return gui.switchFocus(view)
}

// focusPanelItem reports missing async-loaded selectors instead of silently retaining a stale cursor.
func focusPanelItem[T comparable](gui *Gui, panel *panels.SideListPanel[T], ref resources.Ref) error {
	if err := gui.goToPanel(panel.GetView()); err != nil {
		return err
	}

	switch len(ref.Path) {
	case 0:
		return nil
	case 1:
		if !panel.SelectByCell(ref.Path[0]) {
			return fmt.Errorf("no %q in %s (it may still be loading)", ref.Path[0], ref.Key().Name())
		}
		return panel.HandleSelect()
	default:
		return fmt.Errorf("%s addresses one item, got %d selectors", ref.Key().Name(), len(ref.Path))
	}
}

func (gui *Gui) FocusProfiles(ref resources.Ref) error {
	if err := gui.goToPanel(gui.Views.Profile); err != nil {
		return err
	}

	if len(ref.Path) > 1 {
		return fmt.Errorf("a profile ref addresses one profile, got %d selectors", len(ref.Path))
	}
	if len(ref.Path) == 0 {
		return nil
	}

	// Match raw profile items because active rows are decorated.
	profile := ref.Path[0]
	if !gui.Panels.Profile.SelectByItem(profile) {
		return fmt.Errorf("no profile called %q in ~/.aws/config", profile)
	}

	if profile == gui.CurrentProfile {
		return gui.Panels.Profile.HandleSelect()
	}

	return gui.switchProfile(profile)
}

// FocusECS loads drill levels on entry so server results remain authoritative.
func (gui *Gui) FocusECS(ref resources.Ref) error {
	if err := gui.goToPanel(gui.Views.ECS); err != nil {
		return err
	}

	var drill ecsDrillState
	switch len(ref.Path) {
	case 0:
		if gui.ecsDrill.level == ecsLevelClusters {
			return nil
		}
	case 1:
		drill = ecsDrillState{level: ecsLevelServices, cluster: ref.Path[0]}
	case 2:
		drill = ecsDrillState{level: ecsLevelTasks, cluster: ref.Path[0], service: ref.Path[1]}
	default:
		return fmt.Errorf("an ECS ref goes as deep as cluster and service, got %d selectors", len(ref.Path))
	}

	gui.ecsDrill = drill
	gui.Views.ECS.Title = ecsDrillTitle(drill)

	return gui.drillECS()
}

func (gui *Gui) FocusEC2(ref resources.Ref) error {
	return focusPanelItem(gui, gui.Panels.EC2, ref)
}

func (gui *Gui) FocusS3(ref resources.Ref) error {
	return focusPanelItem(gui, gui.Panels.S3, ref)
}

func (gui *Gui) FocusEKS(ref resources.Ref) error {
	return focusPanelItem(gui, gui.Panels.EKS, ref)
}

func (gui *Gui) FocusECR(ref resources.Ref) error {
	return focusPanelItem(gui, gui.Panels.ECR, ref)
}

func (gui *Gui) FocusSecrets(ref resources.Ref) error {
	return focusPanelItem(gui, gui.Panels.Secrets, ref)
}

func (gui *Gui) FocusVPC(ref resources.Ref) error {
	return focusPanelItem(gui, gui.Panels.VPC, ref)
}

func (gui *Gui) FocusAmazonQ(ref resources.Ref) error {
	if len(ref.Path) > 0 {
		return fmt.Errorf("%s takes no selector", ref.Key().Name())
	}
	return gui.handleToggleQ()
}

func (gui *Gui) FocusSettings(ref resources.Ref) error {
	if len(ref.Path) > 0 {
		return fmt.Errorf("%s takes no selector", ref.Key().Name())
	}
	return gui.handleToggleSettings()
}
