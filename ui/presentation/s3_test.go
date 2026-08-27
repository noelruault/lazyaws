package presentation

import (
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

func TestGetBucketDisplayCells(t *testing.T) {
	b := &aws.Bucket{Name: "my-bucket", Region: "eu-west-1", CreationDate: "2026-07-10 00:00:00"}

	wantCells(t, GetBucketDisplayCells(b), []utils.Cell{
		{Text: "my-bucket"},
		{Text: "eu-west-1"},
		// The row compresses the stamp to a glance; the Overview keeps the full timestamp.
		{Text: "10 Jul 00:00", Color: color.Faint},
	})
}

// A bucket name can run to 63 characters, well past a side panel, and the region and creation date are the columns that must survive it.
func TestBucketRowFitsALongNameIntoANarrowPanel(t *testing.T) {
	const width = 40

	b := &aws.Bucket{Name: strings.Repeat("b", 63), Region: "eu-west-1", CreationDate: "2026-07-10 00:00:00"}

	rendered, err := utils.RenderTableFit([][]utils.Cell{GetBucketDisplayCells(b)}, width, BucketWeights())
	if err != nil {
		t.Fatalf("RenderTableFit: %v", err)
	}

	if got := runewidth.StringWidth(rendered); got > width {
		t.Errorf("row is %d cells wide, want at most %d: %q", got, width, rendered)
	}
	if !strings.Contains(rendered, "…") {
		t.Errorf("row = %q, want the name cut with an ellipsis", rendered)
	}
	for _, want := range []string{"eu-west-1", "10 Jul 00:00"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("row = %q, want it to still show %q in full", rendered, want)
		}
	}
}

func TestBucketWeightsMatchTheRowWidth(t *testing.T) {
	if got, want := len(BucketWeights()), len(GetBucketDisplayCells(&aws.Bucket{})); got != want {
		t.Errorf("%d weights for %d cells", got, want)
	}
}
