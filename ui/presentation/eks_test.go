package presentation

import (
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

func TestGetEKSClusterDisplayCells(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cluster *aws.EKSCluster
		want    []utils.Cell
	}{
		{
			"a healthy cluster",
			&aws.EKSCluster{Name: "prod-eks", Status: "ACTIVE", NodeCount: 12, CreatedAt: "2026-01-02 15:04:05"},
			[]utils.Cell{
				{Text: "▶ ACTIVE", Color: color.FgGreen},
				{Text: "prod-eks", Color: color.Bold},
				{Text: "12 nodes"},
				{Text: "2026-01-02", Color: color.Faint},
			},
		},
		{
			// EKS has no UPDATING alias, and the icon alone would render it as "?" — the same glyph a cluster the client could not describe gets.
			"a status the style table does not know keeps its word",
			&aws.EKSCluster{Name: "staging", Status: "UPDATING", CreatedAt: "2026-01-02 15:04:05"},
			[]utils.Cell{
				{Text: "? UPDATING", Color: color.FgWhite},
				{Text: "staging", Color: color.Bold},
				{Text: "0 nodes"},
				{Text: "2026-01-02", Color: color.Faint},
			},
		},
		{
			// GetEKSClusterDetails failing is what puts "unknown" on the row, and the node count is then never fetched.
			"a cluster the client could not describe",
			&aws.EKSCluster{Name: "broken", Status: "unknown"},
			[]utils.Cell{
				{Text: "? unknown", Color: color.FgWhite},
				{Text: "broken", Color: color.Bold},
				{Text: "0 nodes"},
				{Text: "", Color: color.Faint},
			},
		},
		{
			"a deleting cluster",
			&aws.EKSCluster{Name: "old", Status: "DELETING", NodeCount: 3, CreatedAt: "2025-11-30 08:00:00"},
			[]utils.Cell{
				{Text: "− DELETING", Color: color.FgYellow},
				{Text: "old", Color: color.Bold},
				{Text: "3 nodes"},
				{Text: "2025-11-30", Color: color.Faint},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantCells(t, GetEKSClusterDisplayCells(tt.cluster), tt.want)
		})
	}
}

// The whole point of dropping the time of day: at the width a side panel really gets, the date has to still be a date rather than a stamp cut mid-way.
func TestEKSClusterRowKeepsTheNameAndTheWholeDateInANarrowPanel(t *testing.T) {
	forceColor(t)
	const width = 40

	cluster := &aws.EKSCluster{
		Name:      strings.Repeat("c", 60),
		Status:    "ACTIVE",
		NodeCount: 12,
		CreatedAt: "2026-01-02 15:04:05",
	}

	rendered, err := utils.RenderTableFit([][]utils.Cell{GetEKSClusterDisplayCells(cluster)}, width, EKSClusterWeights())
	if err != nil {
		t.Fatalf("RenderTableFit: %v", err)
	}

	plain := utils.Decolorise(rendered)
	if got := runewidth.StringWidth(plain); got > width {
		t.Errorf("row is %d cells wide, want at most %d: %q", got, width, plain)
	}
	if !strings.HasPrefix(plain, "▶ ACTIVE ccc") {
		t.Errorf("row = %q, want the badge and then the name still on screen", plain)
	}
	if !strings.Contains(plain, "…") {
		t.Errorf("row = %q, want the name cut with an ellipsis", plain)
	}
	for _, want := range []string{"12 nodes", "2026-01-02"} {
		if !strings.Contains(plain, want) {
			t.Errorf("row = %q, want it to still show %q whole", plain, want)
		}
	}
}

func TestEKSClusterWeightsMatchTheRowWidth(t *testing.T) {
	if got, want := len(EKSClusterWeights()), len(GetEKSClusterDisplayCells(&aws.EKSCluster{})); got != want {
		t.Errorf("%d weights for %d cells", got, want)
	}
}

func TestCreatedDate(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"2026-01-02 15:04:05", "2026-01-02"},
		{"", ""},
		// A stamp with no space is passed through rather than cut at a fixed offset, so a format change degrades to the old behaviour instead of to a truncated date.
		{"2026-01-02T15:04:05Z", "2026-01-02T15:04:05Z"},
	} {
		if got := createdDate(tt.in); got != tt.want {
			t.Errorf("createdDate(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
