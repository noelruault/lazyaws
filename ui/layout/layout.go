// Package layout splits a terminal rectangle into named windows.
package layout

// Direction is the axis a box lays its children out along.
type Direction int

const (
	// Row stacks children vertically, so each child spans the full width.
	Row Direction = iota
	// Column places children side by side, so each child spans the full height.
	Column
)

// Dimensions is an inclusive rectangle: one cell wide means X1 == X0, and no room at all means X1 == X0-1.
type Dimensions struct {
	X0 int
	X1 int
	Y0 int
	Y1 int
}

// Box is one node of the layout tree, either naming a Window or holding Children, never both.
type Box struct {
	Direction Direction

	// ConditionalDirection wins over Direction, letting a narrow terminal stack what a wide one puts side by side.
	ConditionalDirection func(width int, height int) Direction

	Children []*Box

	// ConditionalChildren wins over Children.
	ConditionalChildren func(width int, height int) []*Box

	Window string

	// Size is honoured only when Weight is zero.
	Size int

	// Weight claims a share of what the fixed-size siblings leave. Shares are relative, so 1:2 and 2:4 divide identically.
	Weight int
}

// Arrange returns the rectangle assigned to each named window.
func Arrange(root *Box, x0, y0, width, height int) map[string]Dimensions {
	windows := map[string]Dimensions{}
	assign(root, x0, y0, width, height, windows)

	return windows
}

func assign(box *Box, x0, y0, width, height int, windows map[string]Dimensions) {
	children := box.Children
	if box.ConditionalChildren != nil {
		children = box.ConditionalChildren(width, height)
	}

	if len(children) == 0 {
		if box.Window != "" {
			windows[box.Window] = Dimensions{X0: x0, X1: x0 + width - 1, Y0: y0, Y1: y0 + height - 1}
		}

		return
	}

	direction := box.Direction
	if box.ConditionalDirection != nil {
		direction = box.ConditionalDirection(width, height)
	}

	available := height
	if direction == Column {
		available = width
	}

	offset := 0
	for i, extent := range divide(children, available) {
		if direction == Column {
			assign(children[i], x0+offset, y0, extent, height, windows)
		} else {
			assign(children[i], x0, y0+offset, width, extent, windows)
		}
		offset += extent
	}
}

// divide gives the indivisible remainder to the earliest weighted children, one cell each, so the extents always sum back to available.
func divide(children []*Box, available int) []int {
	extents := make([]int, len(children))

	weighted := make([]int, 0, len(children))
	totalWeight := 0
	for i, child := range children {
		if child.Weight > 0 {
			weighted = append(weighted, i)
			totalWeight += child.Weight
			continue
		}

		// A fixed child never takes more than is left, or it would draw outside its parent.
		extents[i] = min(child.Size, max(available, 0))
		available -= extents[i]
	}

	// Nothing flexes, so leftover space stays unused rather than being forced onto a sibling.
	if len(weighted) == 0 {
		return extents
	}
	if available < 0 {
		available = 0
	}

	// Reducing by the common divisor keeps the remainder below the number of weighted children, so none is skipped.
	divisor := 0
	for _, i := range weighted {
		divisor = gcd(divisor, children[i].Weight)
	}
	totalShares := totalWeight / divisor

	share := available / totalShares
	remainder := available % totalShares

	for _, i := range weighted {
		extents[i] = children[i].Weight / divisor * share
	}
	// The remainder can exceed the number of weighted children when space is tight, so it wraps rather than stopping at the last one.
	for n := range remainder {
		extents[weighted[n%len(weighted)]]++
	}

	return extents
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}

	return a
}
