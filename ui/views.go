package ui

import (
	"github.com/jesseduffield/gocui"
)

type Views struct {
	Profile *gocui.View
	ECS     *gocui.View
	EC2     *gocui.View
	S3      *gocui.View
	EKS     *gocui.View
	ECR     *gocui.View
	Secrets *gocui.View

	VPC *gocui.View

	Main *gocui.View

	Options      *gocui.View
	Information  *gocui.View
	AppStatus    *gocui.View
	FilterPrefix *gocui.View
	Filter       *gocui.View

	CommandPrefix *gocui.View
	Command       *gocui.View
	CommandHint   *gocui.View

	QChats *gocui.View
	QInput *gocui.View

	Settings *gocui.View

	Confirmation *gocui.View
	Menu         *gocui.View

	Limit *gocui.View
}

type viewNameMapping struct {
	viewPtr **gocui.View
	name    string
	// Popups opt out because they are positioned manually.
	autoPosition bool
}

func (gui *Gui) autoPositionedViewNames() []string {
	names := make([]string, 0, len(gui.orderedViewNameMappings()))
	for _, mapping := range gui.orderedViewNameMappings() {
		if mapping.autoPosition {
			names = append(names, mapping.name)
		}
	}
	return names
}

func (gui *Gui) orderedViewNameMappings() []viewNameMapping {
	return []viewNameMapping{
		{viewPtr: &gui.Views.Profile, name: "profile", autoPosition: true},
		{viewPtr: &gui.Views.ECS, name: "ecs", autoPosition: true},
		{viewPtr: &gui.Views.EC2, name: "ec2", autoPosition: true},
		{viewPtr: &gui.Views.S3, name: "s3", autoPosition: true},
		{viewPtr: &gui.Views.EKS, name: "eks", autoPosition: true},
		{viewPtr: &gui.Views.ECR, name: "ecr", autoPosition: true},
		{viewPtr: &gui.Views.Secrets, name: "secrets", autoPosition: true},
		{viewPtr: &gui.Views.VPC, name: "vpc", autoPosition: true},

		{viewPtr: &gui.Views.Main, name: "main", autoPosition: true},

		{viewPtr: &gui.Views.Options, name: "options", autoPosition: true},
		{viewPtr: &gui.Views.AppStatus, name: "appStatus", autoPosition: true},
		{viewPtr: &gui.Views.Information, name: "information", autoPosition: true},
		{viewPtr: &gui.Views.Filter, name: "filter", autoPosition: true},
		{viewPtr: &gui.Views.FilterPrefix, name: "filterPrefix", autoPosition: true},
		{viewPtr: &gui.Views.Command, name: "command", autoPosition: true},
		{viewPtr: &gui.Views.CommandPrefix, name: "commandPrefix", autoPosition: true},
		{viewPtr: &gui.Views.CommandHint, name: "commandHint", autoPosition: true},

		{viewPtr: &gui.Views.QChats, name: "qChats", autoPosition: true},
		{viewPtr: &gui.Views.QInput, name: "qInput", autoPosition: true},

		{viewPtr: &gui.Views.Settings, name: "settings", autoPosition: true},

		{viewPtr: &gui.Views.Menu, name: "menu", autoPosition: false},
		{viewPtr: &gui.Views.Confirmation, name: "confirmation", autoPosition: false},

		{viewPtr: &gui.Views.Limit, name: "limit", autoPosition: true},
	}
}

// createAllViews runs before layout because gocui cannot position or render missing views.
func (gui *Gui) createAllViews() error {
	frameRunes := []rune{'─', '│', '╭', '╮', '╰', '╯'}
	switch gui.Config.User.Gui.Border {
	case "single":
		frameRunes = []rune{'─', '│', '┌', '┐', '└', '┘'}
	case "double":
		frameRunes = []rune{'═', '║', '╔', '╗', '╚', '╝'}
	case "hidden":
		frameRunes = []rune{' ', ' ', ' ', ' ', ' ', ' '}
	}

	selectedLineBgColor := GetGocuiStyle(gui.Config.User.Gui.Theme.SelectedLineBgColor)
	// Kept for onFocusChange, which hands the bar to whichever list holds focus and takes it back when focus leaves.
	gui.selectedLineBgColor = selectedLineBgColor

	var err error
	for _, mapping := range gui.orderedViewNameMappings() {
		// SetView reports successful creation as ErrUnknownView; every other error is fatal.
		*mapping.viewPtr, err = gui.g.SetView(mapping.name, 0, 0, 10, 10, 0)
		if err != nil && err.Error() != gocui.ErrUnknownView.Error() {
			return err
		}
		(*mapping.viewPtr).FrameRunes = frameRunes
		(*mapping.viewPtr).FgColor = gocui.ColorDefault
	}

	gui.Views.Main.Wrap = gui.Config.User.Gui.WrapMainPanel
	// Interactive containers inject carriage returns that would corrupt log rendering.
	gui.Views.Main.IgnoreCarriageReturns = true

	for i, sp := range []struct {
		view  *gocui.View
		title string
	}{
		{gui.Views.Profile, "Profiles"},
		{gui.Views.ECS, "ECS"},
		{gui.Views.EC2, "EC2"},
		{gui.Views.S3, "S3"},
		{gui.Views.EKS, "EKS"},
		{gui.Views.ECR, "ECR"},
		{gui.Views.Secrets, "Secrets"},
		{gui.Views.VPC, "VPC"},
	} {
		sp.view.Title = sp.title
		sp.view.TitlePrefix = "[" + string(rune('1'+i)) + "]"
		sp.view.Highlight = true
		sp.view.SelBgColor = selectedLineBgColor
	}

	gui.Views.Options.Frame = false

	gui.Views.AppStatus.FgColor = gocui.ColorCyan
	gui.Views.AppStatus.Frame = false

	gui.Views.Information.Frame = false
	gui.Views.Information.FgColor = gocui.ColorGreen

	gui.Views.Confirmation.Visible = false
	gui.Views.Confirmation.Wrap = true

	gui.Views.Menu.Visible = false
	gui.Views.Menu.SelBgColor = selectedLineBgColor

	gui.Views.Limit.Visible = false
	gui.Views.Limit.Title = "Not enough space to render panels"
	gui.Views.Limit.Wrap = true

	gui.Views.FilterPrefix.BgColor = gocui.ColorDefault
	gui.Views.FilterPrefix.FgColor = gocui.ColorGreen
	gui.Views.FilterPrefix.Frame = false

	gui.Views.Filter.BgColor = gocui.ColorDefault
	gui.Views.Filter.FgColor = gocui.ColorGreen
	gui.Views.Filter.Editable = true
	gui.Views.Filter.Frame = false
	gui.Views.Filter.Editor = gocui.EditorFunc(gui.wrapEditor(gocui.SimpleEditor))

	gui.Views.CommandPrefix.BgColor = gocui.ColorDefault
	gui.Views.CommandPrefix.FgColor = gocui.ColorGreen
	gui.Views.CommandPrefix.Frame = false

	gui.Views.Command.BgColor = gocui.ColorDefault
	gui.Views.Command.FgColor = gocui.ColorGreen
	gui.Views.Command.Editable = true
	gui.Views.Command.Frame = false
	gui.Views.Command.Editor = gocui.EditorFunc(gui.wrapEditorWith(gocui.SimpleEditor, gui.onNewCommandInput))

	gui.Views.CommandHint.BgColor = gocui.ColorDefault
	gui.Views.CommandHint.FgColor = gocui.ColorDefault | gocui.AttrDim
	gui.Views.CommandHint.Frame = false

	gui.Views.QChats.Title = "Chats"
	gui.Views.QChats.Highlight = true
	gui.Views.QChats.SelBgColor = selectedLineBgColor

	gui.Views.Settings.Title = "Settings"
	gui.Views.Settings.Highlight = true
	gui.Views.Settings.SelBgColor = selectedLineBgColor

	gui.Views.QInput.Title = "Ask Amazon Q"
	gui.Views.QInput.Editable = true
	gui.Views.QInput.Editor = gocui.EditorFunc(gocui.SimpleEditor)

	_ = gui.setViewContent(gui.Views.FilterPrefix, gui.filterPrompt())
	_ = gui.setViewContent(gui.Views.CommandPrefix, gui.commandPrompt())

	return nil
}
