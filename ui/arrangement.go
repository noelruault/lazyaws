// arrangement.go — the lazyaws port of lazydocker's pkg/gui/arrangement.go (MIT, © 2018 Jesse Duffield).
package ui

import (
	"github.com/mattn/go-runewidth"
	"github.com/noelruault/lazyaws/ui/layout"

	"github.com/noelruault/lazyaws/config"
	"github.com/noelruault/lazyaws/ui/utils"
)

const infoSectionPadding = " "

const qInputHeight = 3

func (gui *Gui) getWindowDimensions(informationStr string, appStatus string) map[string]layout.Dimensions {
	minimumHeight := 9
	minimumWidth := 10
	width, height := gui.g.Size()
	if width < minimumWidth || height < minimumHeight {
		return layout.Arrange(&layout.Box{Window: "limit"}, 0, 0, width, height)
	}

	showInfoSection := gui.Config.User.Gui.ShowBottomLine || gui.State.Filter.active || gui.State.Command.active
	infoSectionSize := 0
	if showInfoSection {
		infoSectionSize = 1
	}

	if gui.State.Settings.active {
		return layout.Arrange(&layout.Box{
			Direction: layout.Row,
			Children: []*layout.Box{
				{Window: "settings", Weight: 1},
				{
					Direction: layout.Column,
					Size:      infoSectionSize,
					Children:  gui.infoSectionChildren(informationStr, appStatus),
				},
			},
		}, 0, 0, width, height)
	}

	if gui.State.Q.active {
		return layout.Arrange(gui.qScreenBox(infoSectionSize, gui.infoSectionChildren(informationStr, appStatus)), 0, 0, width, height)
	}

	sideSectionWeight, mainSectionWeight := gui.getMidSectionWeights()

	sidePanelsDirection := layout.Column
	portraitMode := width <= 84 && height > 45
	if portraitMode {
		sidePanelsDirection = layout.Row
	}

	root := &layout.Box{
		Direction: layout.Row,
		Children: []*layout.Box{
			{
				Direction: sidePanelsDirection,
				Weight:    1,
				Children: []*layout.Box{
					{
						Direction:           layout.Row,
						Weight:              sideSectionWeight,
						ConditionalChildren: gui.sidePanelChildren,
					},
					{
						Window: "main",
						Weight: mainSectionWeight,
					},
				},
			},
			{
				Direction: layout.Column,
				Size:      infoSectionSize,
				Children:  gui.infoSectionChildren(informationStr, appStatus),
			},
		},
	}

	return layout.Arrange(root, 0, 0, width, height)
}

func (gui *Gui) getMidSectionWeights() (int, int) {
	currentWindow := gui.currentStaticViewName()

	ratio := gui.Config.User.Gui.SidePanelWidth
	if ratio <= 0 {
		ratio = config.DefaultUserConfig().Gui.SidePanelWidth
	}
	mainSectionWeight := int(1/ratio) - 1
	sideSectionWeight := 1

	if currentWindow == "main" && gui.State.ScreenMode == SCREEN_FULL {
		mainSectionWeight = 1
		sideSectionWeight = 0
	} else {
		if gui.State.ScreenMode == SCREEN_HALF {
			mainSectionWeight = 1
		} else if gui.State.ScreenMode == SCREEN_FULL {
			mainSectionWeight = 0
		}
	}

	return sideSectionWeight, mainSectionWeight
}

// qScreenBox matches dashboard sidebar width so screen swaps stay aligned.
func (gui *Gui) qScreenBox(infoSectionSize int, infoSectionChildren []*layout.Box) *layout.Box {
	ratio := gui.Config.User.Gui.SidePanelWidth
	if ratio <= 0 {
		ratio = config.DefaultUserConfig().Gui.SidePanelWidth
	}
	conversationWeight := max(int(1/ratio)-1, 1)

	return &layout.Box{
		Direction: layout.Row,
		Children: []*layout.Box{
			{
				Direction: layout.Column,
				Weight:    1,
				Children: []*layout.Box{
					{Window: "qChats", Weight: 1},
					{
						Direction: layout.Row,
						Weight:    conversationWeight,
						Children: []*layout.Box{
							{Window: "main", Weight: 1},
							{Window: "qInput", Size: qInputHeight},
						},
					},
				},
			},
			{
				Direction: layout.Column,
				Size:      infoSectionSize,
				Children:  infoSectionChildren,
			},
		},
	}
}

func (gui *Gui) infoSectionChildren(informationStr string, appStatus string) []*layout.Box {
	result := []*layout.Box{}

	if len(appStatus) > 0 {
		result = append(result,
			&layout.Box{
				Window: "appStatus",
				Size:   runewidth.StringWidth(appStatus) + runewidth.StringWidth(infoSectionPadding),
			},
		)
	}

	// The two bottom-line inputs take over the whole line while they are open, the command bar first: it is the one that can open the filter.
	if gui.State.Command.active {
		return append(result, gui.commandBarBoxes()...)
	}

	if gui.State.Filter.active {
		return append(result, []*layout.Box{
			{
				Window: "filterPrefix",
				Size:   runewidth.StringWidth(gui.filterPrompt()),
			},
			{
				Window: "filter",
				Weight: 1,
			},
		}...)
	}

	result = append(result,
		[]*layout.Box{
			{
				Window: "options",
				Weight: 1,
			},
			{
				Window: "information",
				// ANSI escapes occupy bytes but no terminal cells, so strip them before measuring.
				Size: runewidth.StringWidth(infoSectionPadding) + runewidth.StringWidth(utils.Decolorise(informationStr)),
			},
		}...,
	)

	return result
}

func (gui *Gui) sidePanelChildren(width int, height int) []*layout.Box {
	return sidePanelBoxes(gui.sideViewNames(), gui.currentSideViewName(), gui.State.ScreenMode, height, gui.Config.User.Gui.ExpandFocusedSidePanel)
}

func sidePanelBoxes(sideWindowNames []string, currentWindow string, screenMode WindowMaximisation, height int, expandFocusedSidePanel bool) []*layout.Box {
	if screenMode == SCREEN_FULL || screenMode == SCREEN_HALF {
		fullHeightBox := func(window string) *layout.Box {
			if window == currentWindow {
				return &layout.Box{Window: window, Weight: 1}
			}
			return &layout.Box{Window: window, Size: 0}
		}

		return mapBoxes(sideWindowNames, fullHeightBox)
	} else if height >= 28 {
		accordionBox := func(defaultBox *layout.Box) *layout.Box {
			if expandFocusedSidePanel && defaultBox.Window == currentWindow {
				return &layout.Box{Window: defaultBox.Window, Weight: 2}
			}
			return defaultBox
		}

		if len(sideWindowNames) > 0 && sideWindowNames[0] == "profile" {
			profileBox := &layout.Box{Window: sideWindowNames[0], Size: 3}
			if currentWindow == sideWindowNames[0] {
				profileBox = &layout.Box{Window: sideWindowNames[0], Weight: 2}
			}

			return append([]*layout.Box{profileBox}, mapBoxes(sideWindowNames[1:], func(window string) *layout.Box {
				return accordionBox(&layout.Box{Window: window, Weight: 1})
			})...)
		}

		return mapBoxes(sideWindowNames, func(window string) *layout.Box {
			return accordionBox(&layout.Box{Window: window, Weight: 1})
		})
	} else {
		squashedHeight := 1
		if height >= 21 {
			squashedHeight = 3
		}

		squashedSidePanelBox := func(window string) *layout.Box {
			if window == currentWindow {
				return &layout.Box{Window: window, Weight: 1}
			}
			return &layout.Box{Window: window, Size: squashedHeight}
		}

		return mapBoxes(sideWindowNames, squashedSidePanelBox)
	}
}

// mapBoxes keeps the layout branches to their differences: each supplies only how one window becomes a box.
func mapBoxes(windows []string, build func(string) *layout.Box) []*layout.Box {
	boxes := make([]*layout.Box, len(windows))
	for i, window := range windows {
		boxes[i] = build(window)
	}

	return boxes
}
