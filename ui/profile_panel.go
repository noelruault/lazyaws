// Portions adapted from lazydocker's project panel (MIT, © 2018 Jesse Duffield).
package ui

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/presentation"
	"github.com/noelruault/lazyaws/ui/tasks"
)

func (gui *Gui) getProfilePanel() *panels.SideListPanel[string] {
	return &panels.SideListPanel[string]{
		ContextState: &panels.ContextState[string]{
			GetMainTabs: func() []panels.MainTab[string] {
				return []panels.MainTab[string]{
					{Key: "credentials", Title: "Credentials", Render: gui.renderProfileCredentials},
					{Key: "config", Title: "Config", Render: gui.renderProfileConfig},
				}
			},
			GetItemContextCacheKey: func(profile string) string {
				return "profile-" + profile
			},
		},

		ListPanel: panels.ListPanel[string]{
			List: panels.NewFilteredList[string](),
			View: gui.Views.Profile,
		},
		NoItemsMessage: "no AWS profiles in ~/.aws/config",
		Gui:            gui.intoInterface(),

		Sort: func(a, b string) bool { return a < b },
		GetTableCells: func(profile string) []string {
			region, accountID := "", ""
			if profile == gui.CurrentProfile && gui.Client != nil {
				region = gui.Client.GetRegion()
				accountID = gui.Client.GetAccountID()
			}
			return presentation.GetProfileDisplayStrings(profile, gui.CurrentProfile, region, accountID)
		},
	}
}

func (gui *Gui) refreshProfile() error {
	gui.Panels.Profile.SetItems(listAWSProfiles())

	if gui.CurrentProfile != "" {
		for i, p := range gui.Panels.Profile.List.GetItems() {
			if p == gui.CurrentProfile {
				gui.Panels.Profile.SetSelectedLineIdx(i)
				break
			}
		}
	}

	return gui.Panels.Profile.RerenderList()
}

func (gui *Gui) handleProfileSwitch(g *gocui.Gui, v *gocui.View) error {
	profile, err := gui.Panels.Profile.GetSelectedItem()
	if err != nil {
		return nil
	}
	return gui.switchProfile(profile)
}

// switchProfile leaves client and panel state untouched on failed or superseded connections.
func (gui *Gui) switchProfile(profile string) error {
	gui.Gen++
	gen := gui.Gen

	return gui.WithWaitingStatus("switching profile", func() error {
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		client, err := aws.NewClientWithProfile(timeoutCtx, profile, "")
		if err != nil {
			return err
		}

		return gui.applyProfileSwitch(gen, profile, client)
	})
}

// applyProfileSwitch rejects slow connections superseded by newer profile switches.
func (gui *Gui) applyProfileSwitch(gen int, profile string, client *aws.Client) error {
	if gen != gui.Gen {
		return nil
	}

	gui.Client = client
	gui.CurrentProfile = profile
	gui.authProblem = client.AuthError()
	gui.resetDependentPanelState()

	gui.throttledRefresh.Trigger()
	return nil
}

func (gui *Gui) resetDependentPanelState() {
	gui.State.Panels.Main.ObjectKey = ""

	gui.ecsDrill = ecsDrillState{}
	gui.Views.ECS.Title = "ECS"
	gui.Panels.ECS.SetItems(nil)

	if gui.Panels.EC2 != nil {
		gui.Panels.EC2.SetItems(nil)
	}
	if gui.Panels.S3 != nil {
		gui.Panels.S3.SetItems(nil)
	}
	gui.s3Objects = s3ObjectsState{}
	if gui.Panels.EKS != nil {
		gui.Panels.EKS.SetItems(nil)
	}
	if gui.Panels.ECR != nil {
		gui.Panels.ECR.SetItems(nil)
	}
	if gui.Panels.Secrets != nil {
		gui.Panels.Secrets.SetItems(nil)
	}
	gui.secretsReveal = secretsRevealState{}
	gui.secretsShowDeleted = false

	if gui.Panels.VPC != nil {
		gui.Panels.VPC.SetItems(nil)
	}
	gui.vpcEndpoints = vpcEndpointsState{}
	gui.mainCursorState = mainCursorState{}
}

func (gui *Gui) renderProfileCredentials(profile string) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string {
		if profile != gui.CurrentProfile || gui.Client == nil {
			return "not connected — press enter to switch to this profile"
		}

		lines := []string{
			"Account ID: " + orNone(gui.Client.GetAccountID()),
			"Region: " + orNone(gui.Client.GetRegion()),
		}
		return strings.Join(lines, "\n")
	})
}

func (gui *Gui) renderProfileConfig(profile string) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string {
		return readAWSConfigSection(profile)
	})
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// readAWSConfigSection handles the AWS CLI's special unprefixed default section.
func readAWSConfigSection(profile string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "no ~/.aws/config found"
	}

	data, err := os.ReadFile(filepath.Join(home, ".aws", "config"))
	if err != nil {
		return "no ~/.aws/config found"
	}

	header := "[profile " + profile + "]"
	if profile == "default" {
		header = "[default]"
	}

	var section []string
	inSection := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inSection = trimmed == header
			if inSection {
				section = append(section, line)
			}
			continue
		}
		if inSection && trimmed != "" {
			section = append(section, line)
		}
	}

	if len(section) == 0 {
		return "no config section found for profile " + profile
	}
	return strings.Join(section, "\n")
}

func listAWSProfiles() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	f, err := os.Open(filepath.Join(home, ".aws", "config"))
	if err != nil {
		return nil
	}
	defer f.Close()

	seen := map[string]bool{}
	var profiles []string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
			continue
		}
		line = strings.TrimPrefix(line, "[")
		line = strings.TrimSuffix(line, "]")
		line = strings.TrimSpace(strings.TrimPrefix(line, "profile "))
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		profiles = append(profiles, line)
	}
	if err := scanner.Err(); err != nil {
		return profiles
	}
	sort.Strings(profiles)
	return profiles
}
