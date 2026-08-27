package presentation

import (
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

func TestGetECRRepositoryDisplayCells(t *testing.T) {
	for _, tt := range []struct {
		name string
		repo *aws.ECRRepository
		want []utils.Cell
	}{
		{
			// Immutable is the posture nobody needs alerting to, so it carries no colour.
			"an immutable repository with scanning on",
			&aws.ECRRepository{Name: "svc-api", TagMutability: "IMMUTABLE", ScanOnPush: true},
			[]utils.Cell{
				{Text: "svc-api", Color: color.Bold},
				{Text: "immutable"},
				{Text: "scan on"},
			},
		},
		{
			"a mutable repository is the one worth a colour",
			&aws.ECRRepository{Name: "svc-worker", TagMutability: "MUTABLE"},
			[]utils.Cell{
				{Text: "svc-worker", Color: color.Bold},
				{Text: "● mutable", Color: color.FgYellow},
				{Text: "scan off"},
			},
		},
		{
			"an exclusion list makes the policy partial, not blanket",
			&aws.ECRRepository{Name: "svc-web", TagMutability: "IMMUTABLE_WITH_EXCLUSION"},
			[]utils.Cell{
				{Text: "svc-web", Color: color.Bold},
				{Text: "immutable*"},
				{Text: "scan off"},
			},
		},
		{
			"a mutable repository with exclusions",
			&aws.ECRRepository{Name: "svc-jobs", TagMutability: "MUTABLE_WITH_EXCLUSION", ScanOnPush: true},
			[]utils.Cell{
				{Text: "svc-jobs", Color: color.Bold},
				{Text: "● mutable*", Color: color.FgYellow},
				{Text: "scan on"},
			},
		},
		{
			"a repository DescribeRepositories reported no policy for",
			&aws.ECRRepository{Name: "svc-old"},
			[]utils.Cell{
				{Text: "svc-old", Color: color.Bold},
				{Text: "-"},
				{Text: "scan off"},
			},
		},
		{
			// A value added to the enum after this build shipped is shown as AWS sent it rather than guessed into one of the two buckets.
			"an unknown policy value",
			&aws.ECRRepository{Name: "svc-new", TagMutability: "SOMETHING_ELSE"},
			[]utils.Cell{
				{Text: "svc-new", Color: color.Bold},
				{Text: "SOMETHING_ELSE"},
				{Text: "scan off"},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantCells(t, GetECRRepositoryDisplayCells(tt.repo), tt.want)
		})
	}
}

// Repository names are long and fully qualified, so the narrow panel is the normal case here rather than the edge one.
func TestECRRepositoryRowFitsALongNameIntoANarrowPanel(t *testing.T) {
	forceColor(t)
	const width = 40

	repo := &aws.ECRRepository{Name: strings.Repeat("r", 60), TagMutability: "MUTABLE", ScanOnPush: true}

	rendered, err := utils.RenderTableFit([][]utils.Cell{GetECRRepositoryDisplayCells(repo)}, width, ECRRepositoryWeights())
	if err != nil {
		t.Fatalf("RenderTableFit: %v", err)
	}

	plain := utils.Decolorise(rendered)
	if got := runewidth.StringWidth(plain); got > width {
		t.Errorf("row is %d cells wide, want at most %d: %q", got, width, plain)
	}
	if !strings.HasPrefix(plain, "rrr") {
		t.Errorf("row = %q, want the repository name still on screen", plain)
	}
	if !strings.Contains(plain, "…") {
		t.Errorf("row = %q, want the name cut with an ellipsis", plain)
	}
	for _, want := range []string{"● mutable", "scan on"} {
		if !strings.Contains(plain, want) {
			t.Errorf("row = %q, want it to still show %q", plain, want)
		}
	}
}

func TestECRRepositoryWeightsMatchTheRowWidth(t *testing.T) {
	if got, want := len(ECRRepositoryWeights()), len(GetECRRepositoryDisplayCells(&aws.ECRRepository{})); got != want {
		t.Errorf("%d weights for %d cells", got, want)
	}
}
