package layout

import (
	"testing"
)

func window(name string, size, weight int) *Box {
	return &Box{Window: name, Size: size, Weight: weight}
}

// Extents are golden values taken from the layout this package replaced, so a regression here is a visible shift in every panel boundary.
func TestArrangeSplitsSpace(t *testing.T) {
	for _, tt := range []struct {
		name          string
		root          *Box
		width, height int
		want          map[string]Dimensions
	}{
		{
			"equal weights hand the remainder to the earliest",
			&Box{Direction: Column, Children: []*Box{window("a", 0, 1), window("b", 0, 1), window("c", 0, 1)}},
			10, 1,
			map[string]Dimensions{"a": {0, 3, 0, 0}, "b": {4, 6, 0, 0}, "c": {7, 9, 0, 0}},
		},
		{
			"remainder spreads one cell at a time",
			&Box{Direction: Column, Children: []*Box{
				window("a", 0, 1), window("b", 0, 1), window("c", 0, 1), window("d", 0, 1),
				window("e", 0, 1), window("f", 0, 1), window("g", 0, 1),
			}},
			10, 1,
			map[string]Dimensions{
				"a": {0, 1, 0, 0}, "b": {2, 3, 0, 0}, "c": {4, 5, 0, 0}, "d": {6, 6, 0, 0},
				"e": {7, 7, 0, 0}, "f": {8, 8, 0, 0}, "g": {9, 9, 0, 0},
			},
		},
		{
			"uneven weights split proportionally",
			&Box{Direction: Column, Children: []*Box{window("a", 0, 1), window("b", 0, 3)}},
			9, 1,
			map[string]Dimensions{"a": {0, 2, 0, 0}, "b": {3, 8, 0, 0}},
		},
		{
			"proportional weights are equivalent to their reduced form",
			&Box{Direction: Column, Children: []*Box{window("a", 0, 2), window("b", 0, 4)}},
			10, 1,
			map[string]Dimensions{"a": {0, 3, 0, 0}, "b": {4, 9, 0, 0}},
		},
		{
			"a fixed size is served before the flexible sibling",
			&Box{Direction: Column, Children: []*Box{window("a", 3, 0), window("b", 0, 1)}},
			10, 1,
			map[string]Dimensions{"a": {0, 2, 0, 0}, "b": {3, 9, 0, 0}},
		},
		{
			"a zero-size sibling collapses without shifting the next",
			&Box{Direction: Column, Children: []*Box{window("a", 0, 0), window("b", 0, 1)}},
			10, 1,
			map[string]Dimensions{"a": {0, -1, 0, 0}, "b": {0, 9, 0, 0}},
		},
		{
			"rows stack and columns nest inside them",
			&Box{Direction: Row, Children: []*Box{
				window("top", 0, 1),
				{Direction: Column, Weight: 1, Children: []*Box{window("bl", 0, 1), window("br", 0, 1)}},
			}},
			10, 10,
			map[string]Dimensions{"top": {0, 9, 0, 4}, "bl": {0, 4, 5, 9}, "br": {5, 9, 5, 9}},
		},
		{
			"fixed rows leave the rest to the flexible one",
			&Box{Direction: Row, Children: []*Box{window("a", 2, 0), window("b", 3, 0), window("c", 0, 1)}},
			5, 9,
			map[string]Dimensions{"a": {0, 4, 0, 1}, "b": {0, 4, 2, 4}, "c": {0, 4, 5, 8}},
		},
		{
			"a lone window takes everything",
			window("solo", 0, 1),
			10, 4,
			map[string]Dimensions{"solo": {0, 9, 0, 3}},
		},
		{
			"no space to give",
			&Box{Direction: Column, Children: []*Box{window("a", 0, 1), window("b", 0, 1)}},
			0, 1,
			map[string]Dimensions{"a": {0, -1, 0, 0}, "b": {0, -1, 0, 0}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := Arrange(tt.root, 0, 0, tt.width, tt.height)
			if len(got) != len(tt.want) {
				t.Fatalf("Arrange() returned %d windows, want %d: %v", len(got), len(tt.want), got)
			}
			for name, want := range tt.want {
				if got[name] != want {
					t.Errorf("%s = %+v, want %+v", name, got[name], want)
				}
			}
		})
	}
}

// The layout this package replaced panicked here, which made a fixed-size-only branch unusable.
func TestArrangeLeavesUnclaimedSpaceUnused(t *testing.T) {
	got := Arrange(&Box{Direction: Column, Children: []*Box{window("a", 4, 0)}}, 0, 0, 10, 1)

	if want := (Dimensions{0, 3, 0, 0}); got["a"] != want {
		t.Errorf("a = %+v, want %+v", got["a"], want)
	}
}

// Fixed children asking for more than exists must not draw past the parent's edge.
func TestArrangeClampsOverflowingFixedChildren(t *testing.T) {
	got := Arrange(&Box{Direction: Column, Children: []*Box{
		window("a", 8, 0), window("b", 7, 0), window("c", 0, 1),
	}}, 0, 0, 10, 1)

	want := map[string]Dimensions{
		"a": {0, 7, 0, 0},  // takes its full 8
		"b": {8, 9, 0, 0},  // asked for 7, clamped to the 2 that remain
		"c": {10, 9, 0, 0}, // nothing left, so an empty rectangle at the edge
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s = %+v, want %+v", name, got[name], w)
		}
	}

	for name, d := range got {
		if d.X1 >= 10 {
			t.Errorf("%s ends at x%d, past the parent's last column 9", name, d.X1)
		}
	}
}

func TestArrangeUsesConditionalOverrides(t *testing.T) {
	root := &Box{
		ConditionalDirection: func(width, _ int) Direction {
			if width > 5 {
				return Column
			}

			return Row
		},
		ConditionalChildren: func(width, _ int) []*Box {
			if width > 5 {
				return []*Box{window("wide", 0, 1), window("other", 0, 1)}
			}

			return []*Box{window("narrow", 0, 1)}
		},
	}

	wide := Arrange(root, 0, 0, 10, 4)
	if want := (Dimensions{0, 4, 0, 3}); wide["wide"] != want {
		t.Errorf("wide = %+v, want %+v", wide["wide"], want)
	}

	narrow := Arrange(root, 0, 0, 4, 4)
	if _, ok := narrow["wide"]; ok {
		t.Error("the wide branch rendered in a narrow terminal")
	}
	if want := (Dimensions{0, 3, 0, 3}); narrow["narrow"] != want {
		t.Errorf("narrow = %+v, want %+v", narrow["narrow"], want)
	}
}

// Every cell of the parent must land in exactly one child, or panels overlap or leave gaps.
func TestArrangeExtentsAlwaysSumToTheParent(t *testing.T) {
	weights := [][]int{{1, 1}, {1, 2}, {1, 1, 1}, {2, 3, 5}, {1, 1, 1, 1, 1, 1, 1}, {4, 4}}

	for _, weightSet := range weights {
		for available := range 40 {
			children := make([]*Box, len(weightSet))
			for i, weight := range weightSet {
				children[i] = window("w", 0, weight)
			}

			total := 0
			for _, extent := range divide(children, available) {
				if extent < 0 {
					t.Fatalf("weights %v over %d produced a negative extent", weightSet, available)
				}
				total += extent
			}
			if total != available {
				t.Errorf("weights %v over %d summed to %d", weightSet, available, total)
			}
		}
	}
}
