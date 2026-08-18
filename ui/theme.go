// Portions adapted from lazydocker's theme and gocui helpers (MIT, © 2018 Jesse Duffield).
package ui

import (
	"github.com/jesseduffield/gocui"
)

// gocuiColorMap stays name-only until configuration accepts hex colors.
var gocuiColorMap = map[string]gocui.Attribute{
	"default":   gocui.ColorDefault,
	"black":     gocui.ColorBlack,
	"red":       gocui.ColorRed,
	"green":     gocui.ColorGreen,
	"yellow":    gocui.ColorYellow,
	"blue":      gocui.ColorBlue,
	"magenta":   gocui.ColorMagenta,
	"cyan":      gocui.ColorCyan,
	"white":     gocui.ColorWhite,
	"bold":      gocui.AttrBold,
	"reverse":   gocui.AttrReverse,
	"underline": gocui.AttrUnderline,
}

func GetGocuiAttribute(key string) gocui.Attribute {
	value, present := gocuiColorMap[key]
	if present {
		return value
	}
	return gocui.ColorDefault
}

func GetGocuiStyle(keys []string) gocui.Attribute {
	var attribute gocui.Attribute
	for _, key := range keys {
		attribute |= GetGocuiAttribute(key)
	}
	return attribute
}

func (gui *Gui) GetOptionsPanelTextColor() gocui.Attribute {
	return GetGocuiStyle(gui.Config.User.Gui.Theme.OptionsTextColor)
}

func (gui *Gui) SetColorScheme() error {
	gui.g.FgColor = GetGocuiStyle(gui.Config.User.Gui.Theme.InactiveBorderColor)
	gui.g.SelFgColor = GetGocuiStyle(gui.Config.User.Gui.Theme.ActiveBorderColor)
	gui.g.FrameColor = gui.g.FgColor
	gui.g.SelFrameColor = gui.g.SelFgColor
	return nil
}
