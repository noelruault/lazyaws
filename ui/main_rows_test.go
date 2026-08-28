package ui

import (
	"strings"
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/panels"
)

// clampCursor carries the movement rules the S3 objects cursor used to own; a cursor that escapes its list would index out of range in Enter and Actions.
func TestClampCursor(t *testing.T) {
	cases := []struct {
		name   string
		index  int
		length int
		want   int
	}{
		{"move down from the top", 1, 3, 1},
		{"move up from the top clamps", -1, 3, 0},
		{"move down at the last row clamps", 3, 3, 2},
		{"an empty list stays at zero", 1, 0, 0},
		{"a list that shrank pulls the cursor back", 9, 2, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clampCursor(tc.index, tc.length); got != tc.want {
				t.Errorf("clampCursor(%d, %d) = %d, want %d", tc.index, tc.length, got, tc.want)
			}
		})
	}
}

func TestRenderMainRowsMarksOnlyTheCursor(t *testing.T) {
	rows := &panels.MainRows{
		Header:       "s3://my-bucket/logs/",
		EmptyMessage: "(empty)",
		Cells: [][]string{
			{"[dir]", "2026/", "-", "-", "-"},
			{"", "a.txt", "42 B", "STANDARD", "2026-01-01"},
		},
	}

	out := renderMainRows(rows, 1)

	var cursorLine, otherLine string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "a.txt") {
			cursorLine = line
		}
		if strings.Contains(line, "2026/") {
			otherLine = line
		}
	}

	if !strings.Contains(cursorLine, ">") {
		t.Errorf("cursor row should carry a highlight marker, got %q", cursorLine)
	}
	if strings.Contains(otherLine, ">") {
		t.Errorf("non-cursor row should not carry the highlight marker, got %q", otherLine)
	}
	if !strings.HasPrefix(out, "s3://my-bucket/logs/\n\n") {
		t.Errorf("header should lead, followed by a blank line, got %q", out)
	}
}

func TestRenderMainRowsEmpty(t *testing.T) {
	out := renderMainRows(&panels.MainRows{Header: "s3://b/", EmptyMessage: "(empty)"}, 0)
	if !strings.Contains(out, "(empty)") {
		t.Errorf("empty listing should say so, got %q", out)
	}
	// The header still identifies which list is empty.
	if !strings.Contains(out, "s3://b/") {
		t.Errorf("empty listing should keep its header, got %q", out)
	}
}

// A tab with no header must not lose its first row to the header offset.
func TestRenderMainRowsWithoutHeader(t *testing.T) {
	rows := &panels.MainRows{Cells: [][]string{{"first"}, {"second"}}}
	// Stripped because colour is on for the whole test binary and the cursor row is styled; the prefix under test is the marker, not the escape.
	out := stripANSIForTest(renderMainRows(rows, 0))

	if !strings.HasPrefix(out, "> ") {
		t.Errorf("the first row should be the cursor row, got %q", out)
	}
	if got := mainRowsHeaderOffset(out); got != 0 {
		t.Errorf("headerless content offset = %d, want 0", got)
	}
}

func TestMainRowsHeaderOffset(t *testing.T) {
	withHeader := renderMainRows(&panels.MainRows{Header: "one line", Cells: [][]string{{"a"}}}, 0)
	if got := mainRowsHeaderOffset(withHeader); got != 2 {
		t.Errorf("single-line header offset = %d, want 2 (header plus its blank line)", got)
	}
}

func TestMainRowsLenIsNilSafe(t *testing.T) {
	var rows *panels.MainRows
	if got := rows.Len(); got != 0 {
		t.Errorf("nil MainRows Len() = %d, want 0", got)
	}
}

// navigableMainRows is the gate every arrow key passes through. A prose tab and a drilled-in detail view both have to scroll; treating either as navigable re-renders the pane and wipes what it shows.
func TestNavigableMainRows(t *testing.T) {
	cases := []struct {
		name string
		tab  panels.MainTab[string]
		want bool
	}{
		{
			name: "a prose tab supplies no rows at all",
			tab:  panels.MainTab[string]{Key: "config", Title: "Config"},
		},
		{
			name: "a detail view supplies rows with nothing to walk",
			tab: panels.MainTab[string]{Key: "endpoints", Title: "Endpoints", Rows: func(string) *panels.MainRows {
				return &panels.MainRows{Back: func() error { return nil }}
			}},
		},
		{
			name: "a populated list is navigable",
			tab: panels.MainTab[string]{Key: "endpoints", Title: "Endpoints", Rows: func(string) *panels.MainRows {
				return &panels.MainRows{Cells: [][]string{{"a"}, {"b"}}}
			}},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gui, _ := newHeadlessGui(t)
			gui.Panels.VPC.SetItems([]*aws.VPC{{ID: "vpc-1", CIDR: "10.0.0.0/16"}})
			gui.Panels.VPC.ContextState.GetMainTabs = func() []panels.MainTab[*aws.VPC] {
				return []panels.MainTab[*aws.VPC]{{
					Key:   tc.tab.Key,
					Title: tc.tab.Title,
					Rows: func(*aws.VPC) *panels.MainRows {
						if tc.tab.Rows == nil {
							return nil
						}
						return tc.tab.Rows("")
					},
				}}
			}
			// Focus has moved into main, which is the state the arrow keys actually fire in.
			gui.State.ViewStack = []string{"vpc", "main"}

			if _, got := gui.navigableMainRows(); got != tc.want {
				t.Errorf("navigableMainRows() = %v, want %v", got, tc.want)
			}
		})
	}
}
