package shared

// Viewport holds scrolling state for list-like views.
type Viewport struct {
	Offset int
	Height int
}

// EnsureVisible adjusts the viewport offset to keep the selected item visible.
func EnsureVisible(selectedIndex, listLength int, vp *Viewport) {
	if listLength == 0 || vp == nil || vp.Height <= 0 {
		return
	}
	if selectedIndex < vp.Offset {
		vp.Offset = selectedIndex
	} else if selectedIndex >= vp.Offset+vp.Height {
		vp.Offset = selectedIndex - vp.Height + 1
	}
	maxOffset := listLength - vp.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if vp.Offset > maxOffset {
		vp.Offset = maxOffset
	}
	if vp.Offset < 0 {
		vp.Offset = 0
	}
}

// GetVisibleRange returns start and end indices for the current viewport.
func GetVisibleRange(listLength int, vp Viewport) (int, int) {
	if listLength == 0 || vp.Height <= 0 {
		return 0, 0
	}
	start := vp.Offset
	end := vp.Offset + vp.Height
	if end > listLength {
		end = listLength
	}
	return start, end
}

// Truncate shortens a string to the given width with ellipsis.
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
