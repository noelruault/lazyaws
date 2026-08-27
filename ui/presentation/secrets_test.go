package presentation

import (
	"fmt"
	"strings"
	"testing"
	"time"

	secretsmanagertypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
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

// overviewNow is the instant every overview test relates its timestamps to, so "6h ago" is a fact about the fixture rather than about when the suite ran.
var overviewNow = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

// overviewWidth is wide enough to stay above minTwoColWidth, since the two-column layout is what most of these assert against.
const overviewWidth = 140

// stackedWidth is below minTwoColWidth, where each block is laid out at full width and a section's own lines sit next to each other.
// Assertions about the WORDS a section chooses belong here: above the threshold the two blocks are interleaved line by line and long values are cut to their column, both of which are the layout's job rather than the formatter's.
const stackedWidth = 80

func ptr[T any](v T) *T { return &v }

// plainOverview renders and strips the escapes, because the states this ticket enumerates are about which words appear, not which colour they take.
func plainOverview(t *testing.T, d *aws.SecretDetails, width int) string {
	t.Helper()

	return utils.Decolorise(FormatSecretOverview(d, width, overviewNow))
}

// rotatingSecret is the mid-rotation AWS-managed secret: it exercises the joint AWSCURRENT+AWSPENDING version, a deprecated version with no stages, replication, tags and an owning service all at once.
func rotatingSecret() *aws.SecretDetails {
	created := overviewNow.Add(-400 * 24 * time.Hour)
	changed := overviewNow.Add(-6 * time.Hour)
	next := overviewNow.Add(7 * 24 * time.Hour)

	return &aws.SecretDetails{
		SecretSummary: aws.SecretSummary{
			Name:            "rds!cluster-1a2b3c4d",
			Arn:             "arn:aws:secretsmanager:eu-west-1:111111111111:secret:rds!cluster-1a2b3c4d-AbCdEf",
			Description:     "Secret associated with the primary cluster",
			CreatedAt:       &created,
			LastChanged:     &changed,
			LastRotated:     &changed,
			NextRotation:    &next,
			RotationEnabled: true,
			RotationDays:    7,
			PrimaryRegion:   "eu-west-1",
			OwningService:   "rds",
			KMSKeyID:        "arn:aws:kms:eu-west-1:111111111111:key/e376f2ab",
			Tags:            []secretsmanagertypes.Tag{{Key: ptr("env"), Value: ptr("staging")}},
		},
		Versions: []secretsmanagertypes.SecretVersionsListEntry{
			{VersionId: ptr("1828f4df"), VersionStages: []string{"AWSCURRENT", "AWSPENDING"}, CreatedDate: &changed},
			{VersionId: ptr("9797e2d4"), VersionStages: []string{"AWSPREVIOUS"}, CreatedDate: &created},
			{VersionId: ptr("5c1de9a0"), CreatedDate: &created},
		},
		Replication: []secretsmanagertypes.ReplicationStatusType{
			{Region: ptr("us-east-1"), Status: secretsmanagertypes.StatusTypeInSync},
		},
		ResourcePolicy: `{"Version":"2012-10-17"}`,
	}
}

// A never-rotated secret is the common case on a real account, and it is the one that has to state every absence rather than leave a blank the reader fills in.
func neverRotatedSecret() *aws.SecretDetails {
	created := overviewNow.Add(-30 * 24 * time.Hour)

	return &aws.SecretDetails{
		SecretSummary: aws.SecretSummary{
			Name:      "app/api-key",
			Arn:       "arn:aws:secretsmanager:eu-west-1:111111111111:secret:app/api-key-XyZ123",
			CreatedAt: &created,
		},
	}
}

func TestFormatSecretOverviewStatesEveryAbsence(t *testing.T) {
	got := plainOverview(t, neverRotatedSecret(), stackedWidth)

	for _, want := range []string{
		"Rotation off",
		"Not replicated",
		"Not configured",
		"Description:    none",
		"Owning service: none",
		"KMS key:        none",
		"Next rotation:  none",
		"Last rotated:   none",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview is missing %q:\n%s", want, got)
		}
	}
	// An empty versions list is the one section with nothing to lay out, and a blank there reads as a failed fetch.
	if !strings.Contains(got, "Versions\nnone") {
		t.Errorf("overview does not state that there are no versions:\n%s", got)
	}
	// Tags render under their own heading, so the word alone would also match the Description row above.
	if !strings.Contains(got, "Tags\nnone") {
		t.Errorf("overview does not state that there are no tags:\n%s", got)
	}
}

func TestFormatSecretOverviewRendersTheRotatingSecret(t *testing.T) {
	got := plainOverview(t, rotatingSecret(), overviewWidth)

	for _, want := range []string{
		"Secret",
		"rds!cluster-1a2b3c4d",
		"Rotation every 7d",
		"eu-west-1",
		"Owning service: rds",
		"Description:    Secret associated with the primary cluster",
		"us-east-1  ▶ InSync",
		"env: staging",
		"Configured, shown on the Config tab",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview is missing %q:\n%s", want, got)
		}
	}
}

// A timestamp needs both halves: the absolute answers "when", the relative answers "is this overdue".
func TestFormatSecretOverviewPairsTimestampsWithTheirDistance(t *testing.T) {
	got := plainOverview(t, rotatingSecret(), overviewWidth)

	if want := "Last changed:   2026-08-27T06:00:00Z (6h ago)"; !strings.Contains(got, want) {
		t.Errorf("overview is missing %q:\n%s", want, got)
	}
	// A future timestamp related the wrong way round reports a rotation that already happened.
	if want := "Next rotation:  2026-09-03T12:00:00Z (in 7d)"; !strings.Contains(got, want) {
		t.Errorf("overview is missing %q:\n%s", want, got)
	}
}

// One version holds AWSCURRENT and AWSPENDING together for the length of a rotation, and a deprecated version holds none at all.
func TestFormatSecretOverviewRendersEveryStageOfAVersion(t *testing.T) {
	got := plainOverview(t, rotatingSecret(), overviewWidth)

	if !strings.Contains(got, "AWSCURRENT AWSPENDING") {
		t.Errorf("the mid-rotation version does not show both of its stages:\n%s", got)
	}
	if strings.Contains(got, "[]") {
		t.Errorf("a version with no stages rendered as an empty list:\n%s", got)
	}
	if !strings.Contains(got, "5c1de9a0 -") {
		t.Errorf("the deprecated version does not render its absent stages as \"-\":\n%s", got)
	}
}

func TestFormatSecretOverviewColoursAVersionByItsLiveStage(t *testing.T) {
	for _, tt := range []struct {
		name   string
		stages []string
		want   utils.Cell
	}{
		{"the live version", []string{"AWSCURRENT"}, utils.Cell{Text: "AWSCURRENT", Color: color.FgGreen}},
		// Mid-rotation the version is still the live one, so it keeps the live colour and the pending label stays legible beside it.
		{"mid-rotation", []string{"AWSCURRENT", "AWSPENDING"}, utils.Cell{Text: "AWSCURRENT AWSPENDING", Color: color.FgGreen}},
		{"a rotation in flight on its own version", []string{"AWSPENDING"}, utils.Cell{Text: "AWSPENDING", Color: color.FgYellow}},
		{"superseded", []string{"AWSPREVIOUS"}, utils.Cell{Text: "AWSPREVIOUS", Color: color.Faint}},
		{"deprecated", nil, utils.Cell{Text: "-", Color: color.Faint}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := secretStagesCell(tt.stages); got != tt.want {
				t.Errorf("secretStagesCell(%v) = %+v, want %+v", tt.stages, got, tt.want)
			}
		})
	}
}

// A secret rotated on a short cadence accumulates versions without bound, and the overview is a glance.
func TestFormatSecretOverviewCapsTheVersionTable(t *testing.T) {
	created := overviewNow.Add(-time.Hour)

	d := neverRotatedSecret()
	for i := range secretVersionsShown + 4 {
		d.Versions = append(d.Versions, secretsmanagertypes.SecretVersionsListEntry{
			VersionId:   ptr(fmt.Sprintf("version-%02d", i)),
			CreatedDate: &created,
		})
	}

	got := plainOverview(t, d, overviewWidth)

	if !strings.Contains(got, "version-14") {
		t.Errorf("the last version inside the cap is missing:\n%s", got)
	}
	if strings.Contains(got, "version-15") {
		t.Errorf("a version past the cap was rendered:\n%s", got)
	}
	if want := "(4 more on the Config tab)"; !strings.Contains(got, want) {
		t.Errorf("overview is missing %q, so the hidden versions are silently dropped:\n%s", want, got)
	}
}

// A secret awaiting deletion reports every other field unchanged, so without this line the pane describes a healthy secret that is about to disappear.
func TestFormatSecretOverviewSaysWhenASecretIsPendingDeletion(t *testing.T) {
	deleted := overviewNow.Add(29 * 24 * time.Hour)

	d := neverRotatedSecret()
	if got := plainOverview(t, d, stackedWidth); strings.Contains(got, "pending deletion") {
		t.Fatalf("a live secret claims to be pending deletion:\n%s", got)
	}

	d.DeletedDate = &deleted
	got := plainOverview(t, d, stackedWidth)
	if want := "pending deletion, deletes 2026-09-25T12:00:00Z (in 29d)"; !strings.Contains(got, want) {
		t.Errorf("overview is missing %q:\n%s", want, got)
	}
}

// The Overview tab renders with wrapping off, so a line wider than the pane is clipped at the edge with nothing to say it was clipped.
func TestFormatSecretOverviewFitsTheWidthItIsGiven(t *testing.T) {
	forceColor(t)

	for _, d := range []*aws.SecretDetails{rotatingSecret(), neverRotatedSecret()} {
		// Either side of minTwoColWidth, because the two-column and the stacked layouts budget their lines differently.
		for width := 40; width <= 200; width++ {
			for _, line := range strings.Split(FormatSecretOverview(d, width, overviewNow), "\n") {
				if cells := runewidth.StringWidth(utils.Decolorise(line)); cells > width {
					t.Fatalf("width %d: line %q is %d cells wide", width, line, cells)
				}
			}
		}
	}
}

// The overview re-renders on a ticker, so anything it touched would be read over and over; it is built from metadata only and the value must never reach it.
func TestFormatSecretOverviewNeverRendersTheValue(t *testing.T) {
	d := rotatingSecret()
	d.ValueString = "hunter2-the-actual-password"

	if got := plainOverview(t, d, overviewWidth); strings.Contains(got, d.ValueString) {
		t.Errorf("the secret's value reached the overview:\n%s", got)
	}
}

func TestSecretRotationBadge(t *testing.T) {
	forceColor(t)

	for _, tt := range []struct {
		name   string
		secret *aws.SecretSummary
		want   string
	}{
		{"never rotated", &aws.SecretSummary{}, utils.ColoredString("Rotation off", color.Faint)},
		{"a seven day cadence", &aws.SecretSummary{RotationEnabled: true, RotationDays: 7}, utils.ColoredString("Rotation every 7d", color.FgGreen)},
		// A secret scheduled by a cron() or rate() expression carries no day cadence until it has rotated once, and "every 0d" would be a cadence nobody configured.
		{"on with no cadence reported yet", &aws.SecretSummary{RotationEnabled: true}, utils.ColoredString("Rotation on", color.FgGreen)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := secretRotationBadge(tt.secret); got != tt.want {
				t.Errorf("secretRotationBadge = %q, want %q", got, tt.want)
			}
		})
	}
}
