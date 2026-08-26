package presentation

import (
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

func TestGetSecretDisplayCells(t *testing.T) {
	deleted := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		name   string
		secret *aws.SecretSummary
		want   []utils.Cell
	}{
		{
			// The common case: a secret that has never had rotation configured carries no RotationRules at all.
			"a secret that does not rotate",
			&aws.SecretSummary{Name: "db-password"},
			[]utils.Cell{
				{Text: "db-password", Color: color.Bold},
				{Text: "rotation off"},
				{Text: "-"},
			},
		},
		{
			"a secret on a seven day cadence",
			&aws.SecretSummary{Name: "db-password", RotationEnabled: true, RotationDays: 7},
			[]utils.Cell{
				{Text: "db-password", Color: color.Bold},
				{Text: "rotation 7d"},
				{Text: "-"},
			},
		},
		{
			// Rotation scheduled by a cron() or rate() expression has no day cadence until it has rotated once, and "rotation 0d" would be a cadence nobody configured.
			"rotation on with no cadence reported yet",
			&aws.SecretSummary{Name: "api-key", RotationEnabled: true},
			[]utils.Cell{
				{Text: "api-key", Color: color.Bold},
				{Text: "rotation on"},
				{Text: "-"},
			},
		},
		{
			"a secret pending deletion",
			&aws.SecretSummary{Name: "old-key", DeletedDate: &deleted},
			[]utils.Cell{
				{Text: "old-key", Color: color.Bold},
				{Text: "rotation off"},
				{Text: "pending deletion", Color: color.FgRed},
			},
		},
		{
			// A secret scheduled for deletion keeps whatever rotation configuration it had, so the two columns are independent.
			"a rotating secret pending deletion",
			&aws.SecretSummary{Name: "old-rotating", RotationEnabled: true, RotationDays: 30, DeletedDate: &deleted},
			[]utils.Cell{
				{Text: "old-rotating", Color: color.Bold},
				{Text: "rotation 30d"},
				{Text: "pending deletion", Color: color.FgRed},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantCells(t, GetSecretDisplayCells(tt.secret), tt.want)
		})
	}
}

// Secret names are paths, so the widest row is the normal one and both right-hand columns have to survive it.
func TestSecretRowFitsALongNameIntoANarrowPanel(t *testing.T) {
	forceColor(t)
	const width = 40

	deleted := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	secret := &aws.SecretSummary{
		Name:            "stage/" + strings.Repeat("s", 60),
		RotationEnabled: true,
		RotationDays:    30,
		DeletedDate:     &deleted,
	}

	rendered, err := utils.RenderTableFit([][]utils.Cell{GetSecretDisplayCells(secret)}, width, SecretWeights())
	if err != nil {
		t.Fatalf("RenderTableFit: %v", err)
	}

	plain := utils.Decolorise(rendered)
	if got := runewidth.StringWidth(plain); got > width {
		t.Errorf("row is %d cells wide, want at most %d: %q", got, width, plain)
	}
	if !strings.HasPrefix(plain, "stage/s") {
		t.Errorf("row = %q, want the secret name still on screen", plain)
	}
	if !strings.Contains(plain, "rotation 30d") {
		t.Errorf("row = %q, want the cadence kept whole", plain)
	}
}

func TestSecretWeightsMatchTheRowWidth(t *testing.T) {
	if got, want := len(SecretWeights()), len(GetSecretDisplayCells(&aws.SecretSummary{})); got != want {
		t.Errorf("%d weights for %d cells", got, want)
	}
}
