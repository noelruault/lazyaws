package ui

import (
	"testing"

	"github.com/noelruault/lazyaws/ui/layout"

	"github.com/noelruault/lazyaws/config"
)

func findBox(t *testing.T, boxes []*layout.Box, window string) *layout.Box {
	t.Helper()
	for _, b := range boxes {
		if b.Window == window {
			return b
		}
	}
	t.Fatalf("no box for window %q in %+v", window, boxes)
	return nil
}

func TestSidePanelBoxesProfileCompactWhenUnfocused(t *testing.T) {
	boxes := sidePanelBoxes([]string{"profile", "ec2", "s3"}, "ec2", SCREEN_NORMAL, 30, true)

	profile := findBox(t, boxes, "profile")
	if profile.Size != 3 || profile.Weight != 0 {
		t.Fatalf("unfocused profile panel should be fixed Size:3, got %+v", profile)
	}

	ec2 := findBox(t, boxes, "ec2")
	if ec2.Weight != 2 {
		t.Fatalf("focused panel should get accordion weight 2, got %+v", ec2)
	}
}

func TestSidePanelBoxesAccordionDisabledKeepsWeightOne(t *testing.T) {
	boxes := sidePanelBoxes([]string{"profile", "ec2", "s3"}, "ec2", SCREEN_NORMAL, 30, false)

	ec2 := findBox(t, boxes, "ec2")
	if ec2.Weight != 1 {
		t.Fatalf("focused panel should stay Weight:1 when accordion is disabled, got %+v", ec2)
	}
}

func TestSidePanelBoxesProfileExpandsWhenFocused(t *testing.T) {
	boxes := sidePanelBoxes([]string{"profile", "ec2"}, "profile", SCREEN_NORMAL, 30, true)

	profile := findBox(t, boxes, "profile")
	if profile.Weight != 2 || profile.Size != 0 {
		t.Fatalf("focused profile panel should expand to Weight:2, got %+v", profile)
	}
}

func TestSidePanelBoxesSquashedModeBelow28Rows(t *testing.T) {
	boxes := sidePanelBoxes([]string{"profile", "ec2", "s3"}, "s3", SCREEN_NORMAL, 20, true)

	profile := findBox(t, boxes, "profile")
	if profile.Size != 1 {
		t.Fatalf("squashed unfocused panel below 21 rows should shrink to Size:1, got %+v", profile)
	}

	s3 := findBox(t, boxes, "s3")
	if s3.Weight != 1 {
		t.Fatalf("focused panel stays Weight:1 in squashed mode, got %+v", s3)
	}
}

func TestSidePanelBoxesSquashedModeBetween21And27Rows(t *testing.T) {
	boxes := sidePanelBoxes([]string{"profile", "ec2"}, "ec2", SCREEN_NORMAL, 25, true)

	profile := findBox(t, boxes, "profile")
	if profile.Size != 3 {
		t.Fatalf("squashed unfocused panel at height 25 should be Size:3, got %+v", profile)
	}
}

func TestSidePanelBoxesFullScreenModeHidesUnfocused(t *testing.T) {
	boxes := sidePanelBoxes([]string{"profile", "ec2", "s3"}, "ec2", SCREEN_FULL, 40, true)

	ec2 := findBox(t, boxes, "ec2")
	if ec2.Weight != 1 {
		t.Fatalf("focused panel takes full weight in SCREEN_FULL, got %+v", ec2)
	}

	profile := findBox(t, boxes, "profile")
	if profile.Size != 0 || profile.Weight != 0 {
		t.Fatalf("unfocused panels collapse to Size:0 in SCREEN_FULL, got %+v", profile)
	}
}

func TestGetMidSectionWeightsFullScreenOnMain(t *testing.T) {
	cfg := &config.Config{User: config.DefaultUserConfig()}
	gui := &Gui{Config: cfg, State: guiState{ViewStack: []string{"main"}, ScreenMode: SCREEN_FULL}}

	side, main := gui.getMidSectionWeights()
	if side != 0 || main != 1 {
		t.Fatalf("SCREEN_FULL on main should give side:0 main:1, got side:%d main:%d", side, main)
	}
}

func TestGetMidSectionWeightsHalfScreen(t *testing.T) {
	cfg := &config.Config{User: config.DefaultUserConfig()}
	gui := &Gui{Config: cfg, State: guiState{ViewStack: []string{"ec2"}, ScreenMode: SCREEN_HALF}}

	side, main := gui.getMidSectionWeights()
	if side != 1 || main != 1 {
		t.Fatalf("SCREEN_HALF should give side:1 main:1, got side:%d main:%d", side, main)
	}
}

// Panels are addressed by row, so a gap or an overlap anywhere in the side column puts every
// click below it on the wrong panel. This walks every configuration the app can produce.
func TestSidePanelsTileTheColumnExactly(t *testing.T) {
	names := []string{"profile", "ecs", "ec2", "s3", "eks", "ecr", "secrets"}
	modes := []WindowMaximisation{SCREEN_NORMAL, SCREEN_HALF, SCREEN_FULL}

	checked := 0
	for _, height := range []int{18, 21, 24, 28, 30, 40, 50, 60} {
		for _, mode := range modes {
			for _, current := range names {
				for _, expand := range []bool{false, true} {
					boxes := sidePanelBoxes(names, current, mode, height, expand)
					windows := layout.Arrange(&layout.Box{Direction: layout.Row, Children: boxes}, 0, 0, 40, height)
					checked++

					next := 0
					for _, name := range names {
						d, ok := windows[name]
						if !ok {
							t.Fatalf("h=%d mode=%d current=%s expand=%v: %s is missing", height, mode, current, expand, name)
						}
						if d.Y0 != next {
							t.Fatalf("h=%d mode=%d current=%s expand=%v: %s starts at y%d, want y%d",
								height, mode, current, expand, name, d.Y0, next)
						}
						next = d.Y1 + 1
					}
					if next != height {
						t.Fatalf("h=%d mode=%d current=%s expand=%v: panels covered %d rows, want %d",
							height, mode, current, expand, next, height)
					}
				}
			}
		}
	}

	if checked != 336 {
		t.Errorf("checked %d configurations, want the full 336-configuration matrix", checked)
	}
}
