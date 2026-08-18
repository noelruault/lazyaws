package panels

import "testing"

func TestListPanelSetSelectedLineIdxClamps(t *testing.T) {
	p := &ListPanel[int]{List: NewFilteredList[int]()}
	p.List.SetItems([]int{1, 2, 3})

	cases := []struct{ in, want int }{
		{-5, 0}, {0, 0}, {2, 2}, {3, 2}, {99, 2},
	}
	for _, c := range cases {
		p.SetSelectedLineIdx(c.in)
		if p.SelectedIdx != c.want {
			t.Errorf("SetSelectedLineIdx(%d) = %d, want %d", c.in, p.SelectedIdx, c.want)
		}
	}

	p.SetSelectedLineIdx(2)
	p.SelectNextLine()
	if p.SelectedIdx != 2 {
		t.Errorf("SelectNextLine past end = %d, want 2", p.SelectedIdx)
	}
	p.SelectPrevLine()
	if p.SelectedIdx != 1 {
		t.Errorf("SelectPrevLine = %d, want 1", p.SelectedIdx)
	}

	p.List.SetItems(nil)
	p.SetSelectedLineIdx(5)
	if p.SelectedIdx != 0 {
		t.Errorf("empty-list SelectedIdx = %d, want 0", p.SelectedIdx)
	}
}

// Selector matching uses rendered cells so panels need no second identity mapping.
func TestSelectByCell(t *testing.T) {
	newPanel := func() *SideListPanel[string] {
		panel := &SideListPanel[string]{
			ListPanel:     ListPanel[string]{List: NewFilteredList[string]()},
			GetTableCells: func(item string) []string { return []string{item, "cell-for-" + item} },
		}
		panel.List.SetItems([]string{"bastion", "web-server-1", "worker"})
		return panel
	}

	panel := newPanel()
	if !panel.SelectByCell("web-server-1") {
		t.Fatal("SelectByCell did not find web-server-1")
	}
	if panel.SelectedIdx != 1 {
		t.Errorf("SelectedIdx = %d, want 1", panel.SelectedIdx)
	}

	panel = newPanel()
	if !panel.SelectByCell("cell-for-worker") {
		t.Fatal("SelectByCell did not match on a non-first cell")
	}
	if panel.SelectedIdx != 2 {
		t.Errorf("SelectedIdx = %d, want 2", panel.SelectedIdx)
	}

	// Misses must preserve selection instead of moving the cursor arbitrarily.
	panel = newPanel()
	panel.SetSelectedLineIdx(2)
	if panel.SelectByCell("no-such-thing") {
		t.Error("SelectByCell claimed to find something that is not there")
	}
	if panel.SelectedIdx != 2 {
		t.Errorf("a miss moved the cursor to %d", panel.SelectedIdx)
	}

	empty := &SideListPanel[string]{
		ListPanel:     ListPanel[string]{List: NewFilteredList[string]()},
		GetTableCells: func(item string) []string { return []string{item} },
	}
	if empty.SelectByCell("anything") {
		t.Error("SelectByCell found something in an empty panel")
	}
}
