package presentation

import (
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

func TestGetInstanceDisplayCells(t *testing.T) {
	inst := &aws.Instance{
		ID:           "i-0123456789abcdef0",
		Name:         "web-1",
		State:        "running",
		InstanceType: "t3.micro",
		PrivateIP:    "10.0.0.5",
	}

	wantCells(t, GetInstanceDisplayCells(inst), []utils.Cell{
		{Text: "▶", Color: color.FgGreen},
		{Text: "web-1", Color: color.Bold},
		{Text: "i-0123456789abcdef0", Color: color.Faint},
		{Text: "t3.micro"},
		{Text: "10.0.0.5"},
	})
}

// An instance with no Name tag still has to be selectable, so the column says so rather than going blank.
func TestGetInstanceDisplayCellsUnnamed(t *testing.T) {
	inst := &aws.Instance{ID: "i-abc", State: "stopped"}

	wantCells(t, GetInstanceDisplayCells(inst), []utils.Cell{
		{Text: "⨯", Color: color.FgRed},
		{Text: "(no name)", Color: color.Bold},
		{Text: "i-abc", Color: color.Faint},
		{Text: ""},
		{Text: ""},
	})
}

// The row a narrow terminal has to survive: a name far wider than the panel must be cut, not allowed to push the instance id and type off the edge.
func TestInstanceRowFitsALongNameIntoANarrowPanel(t *testing.T) {
	forceColor(t)
	const width = 40

	inst := &aws.Instance{
		ID:           "i-0123456789abcdef0",
		Name:         strings.Repeat("n", 60),
		State:        "running",
		InstanceType: "t3.micro",
		PrivateIP:    "10.0.0.5",
	}

	rendered, err := utils.RenderTableFit([][]utils.Cell{GetInstanceDisplayCells(inst)}, width, InstanceWeights())
	if err != nil {
		t.Fatalf("RenderTableFit: %v", err)
	}

	plain := utils.Decolorise(rendered)
	if got := runewidth.StringWidth(plain); got > width {
		t.Errorf("row is %d cells wide, want at most %d: %q", got, width, plain)
	}
	if !strings.Contains(plain, "…") {
		t.Errorf("row = %q, want the name cut with an ellipsis", plain)
	}
	// The name is what you read the list by, so it must survive a squeeze that a purely content-sized layout would have spent entirely on the columns to its right.
	if !strings.HasPrefix(plain, "▶ nnn") {
		t.Errorf("row = %q, want the name still on screen after the icon", plain)
	}
	for _, want := range []string{"i-0", "t3.micro", "10.0.0.5"} {
		if !strings.Contains(plain, want) {
			t.Errorf("row = %q, want it to still show %q", plain, want)
		}
	}
	if strings.Contains(rendered, "\x1b[1m"+strings.Repeat("n", 60)) {
		t.Errorf("row = %q, want the name coloured after the cut, not the full name emitted", rendered)
	}
}

func TestInstanceWeightsMatchTheRowWidth(t *testing.T) {
	if got, want := len(InstanceWeights()), len(GetInstanceDisplayCells(&aws.Instance{})); got != want {
		t.Errorf("%d weights for %d cells", got, want)
	}
}
