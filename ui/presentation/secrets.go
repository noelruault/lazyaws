package presentation

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// SecretWeights gives the secret name the slack; both columns to its right are fixed phrases.
func SecretWeights() []int {
	return []int{1, 0, 0}
}

// GetSecretDisplayCells never touches plaintext.
func GetSecretDisplayCells(s *aws.SecretSummary) []utils.Cell {
	status := utils.Cell{Text: "-"}
	if s.DeletedDate != nil {
		status = utils.Cell{Text: "pending deletion", Color: color.FgRed}
	}

	return []utils.Cell{
		{Text: s.Name, Color: color.Bold},
		rotationCell(s),
		status,
	}
}

// rotationCell labels its own value because the column has no header: "7d" alone in a list of secrets reads as an age.
// Rotation that is on with no day cadence is reported as on rather than as a number, since a secret scheduled by expression carries no AutomaticallyAfterDays until it has rotated once, and printing "rotation 0d" there would invent a cadence.
func rotationCell(s *aws.SecretSummary) utils.Cell {
	switch {
	case !s.RotationEnabled:
		return utils.Cell{Text: "rotation off"}
	case s.RotationDays > 0:
		return utils.Cell{Text: fmt.Sprintf("rotation %dd", s.RotationDays)}
	default:
		return utils.Cell{Text: "rotation on"}
	}
}

// secretVersionsShown caps the version table, because a secret rotated on a short cadence accumulates versions without bound and the overview is a glance, not the history.
const secretVersionsShown = 15

// FormatSecretOverview lays a secret's metadata out for the Overview tab: the header, a two-column body, then the version history that rotation is actually visible in.
// Everything rendered here comes from the DescribeSecret / ListSecretVersionIds / GetResourcePolicy fetch the Config tab already makes, so the tab costs no additional AWS call, and the value itself is never read.
func FormatSecretOverview(d *aws.SecretDetails, width int, now time.Time) string {
	header := ResourceHeader("Secret", d.Name, secretRotationBadge(&d.SecretSummary), "", d.PrimaryRegion)
	body := Columns(width, 2, secretDetailsBlock(d, now), secretPostureBlock(d))

	return header + "\n\n" + body + "\n\n" + secretVersionsBlock(d, width, now)
}

// secretRotationBadge speaks the same vocabulary as the left panel's rotation column, including its rule that a cadence of zero days is an unreported cadence rather than a configured one.
func secretRotationBadge(s *aws.SecretSummary) string {
	switch {
	case !s.RotationEnabled:
		return utils.ColoredString("Rotation off", color.Faint)
	case s.RotationDays > 0:
		return utils.ColoredString(fmt.Sprintf("Rotation every %dd", s.RotationDays), color.FgGreen)
	default:
		return utils.ColoredString("Rotation on", color.FgGreen)
	}
}

func secretDetailsBlock(d *aws.SecretDetails, now time.Time) string {
	rows := []kv{
		{"ARN", orNone(d.Arn)},
		{"Created", secretTime(d.CreatedAt, now)},
		{"Last changed", secretTime(d.LastChanged, now)},
		{"Last rotated", secretTime(d.LastRotated, now)},
		{"Next rotation", secretTime(d.NextRotation, now)},
		{"KMS key", orNone(d.KMSKeyID)},
		{"Description", orNone(d.Description)},
		{"Owning service", orNone(d.OwningService)},
		{"Console", orNone(secretConsoleURL(d))},
	}
	// A secret awaiting deletion still reports every field above unchanged, so without this line the pane describes a healthy secret that is about to disappear.
	if d.DeletedDate != nil {
		rows = append(rows, kv{"Status", utils.ColoredString("pending deletion, deletes "+secretTime(d.DeletedDate, now), color.FgRed)})
	}

	return SectionTitle("Details") + "\n" + kvBlock(rows)
}

func secretPostureBlock(d *aws.SecretDetails) string {
	lines := []string{SectionTitle("Replication")}
	if len(d.Replication) == 0 {
		lines = append(lines, "Not replicated")
	}
	for _, replica := range d.Replication {
		lines = append(lines, orNone(deref(replica.Region))+"  "+Badge(string(replica.Status)))
	}

	lines = append(lines, "", secretPolicyBlock(d))

	lines = append(lines, "", SectionTitle("Tags"))
	if len(d.Tags) == 0 {
		lines = append(lines, "none")
	}
	for _, tag := range d.Tags {
		lines = append(lines, TagLine(orNone(deref(tag.Key)), deref(tag.Value)))
	}

	return strings.Join(lines, "\n")
}

// secretPolicyBlock keeps a policy that could not be read apart from a secret that has none.
// GetResourcePolicy is the last of the three calls the details fetch spends one deadline on, so a timeout, a throttle or a denial reaching here is not evidence of an absence, and "Not configured" would be the pane inventing the safest of the two answers.
func secretPolicyBlock(d *aws.SecretDetails) string {
	if d.ResourcePolicyErr != nil {
		return sectionUnavailable("Resource policy", d.ResourcePolicyErr)
	}

	policy := "Not configured"
	if d.ResourcePolicy != "" {
		policy = "Configured, shown on the Policy tab"
	}

	return SectionTitle("Resource policy") + "\n" + policy
}

// secretConsoleURL rebuilds what the Config tab used to compute, empty when the region never loaded.
func secretConsoleURL(d *aws.SecretDetails) string {
	if d.PrimaryRegion == "" {
		return ""
	}

	return fmt.Sprintf("https://%s.console.aws.amazon.com/secretsmanager/secret?name=%s&region=%s", d.PrimaryRegion, d.Name, d.PrimaryRegion)
}

func secretVersionsBlock(d *aws.SecretDetails, width int, now time.Time) string {
	title := SectionTitle("Versions")
	if len(d.Versions) == 0 {
		return title + "\nnone"
	}

	shown := d.Versions
	if len(shown) > secretVersionsShown {
		shown = shown[:secretVersionsShown]
	}

	rows := make([][]utils.Cell, len(shown))
	for i, version := range shown {
		rows[i] = []utils.Cell{
			{Text: orNone(deref(version.VersionId))},
			secretStagesCell(version.VersionStages),
			{Text: RelTime(derefTime(version.CreatedDate), now), Color: color.Faint},
		}
	}

	// Every column here holds a value of its own natural width, so none of them takes a weight; the rows and the weights are built together, which is why neither error RenderTableFit reports can happen.
	table, _ := utils.RenderTableFit(rows, width, []int{0, 0, 0})

	out := title + "\n" + table
	if hidden := len(d.Versions) - len(shown); hidden > 0 {
		out += "\n" + utils.ColoredString(fmt.Sprintf("(%d more on the Versions tab)", hidden), color.Faint)
	}

	return out
}

// FormatSecretVersions is the Versions tab: the same table as the Overview's, uncapped, for the rotation history the glance deliberately cuts.
func FormatSecretVersions(d *aws.SecretDetails, width int, now time.Time) string {
	if len(d.Versions) == 0 {
		return "none\n"
	}

	rows := make([][]utils.Cell, len(d.Versions))
	for i, version := range d.Versions {
		rows[i] = []utils.Cell{
			{Text: orNone(deref(version.VersionId))},
			secretStagesCell(version.VersionStages),
			{Text: RelTime(derefTime(version.CreatedDate), now), Color: color.Faint},
		}
	}
	table, _ := utils.RenderTableFit(rows, width, []int{0, 0, 0})

	return table
}

// secretStagesCell joins a version's staging labels into one cell.
// A version really does hold several at once — AWSCURRENT and AWSPENDING sit on the same version for the length of a rotation — and a deprecated version holds none at all, which reads as "-" rather than as an empty list.
func secretStagesCell(stages []string) utils.Cell {
	if len(stages) == 0 {
		return utils.Cell{Text: "-", Color: color.Faint}
	}

	// The staging labels are free-form strings in the API, not an enum the SDK exports, so the two AWS reserves are matched literally.
	// The precedence is what a joint AWSCURRENT+AWSPENDING version needs: it is the live version and is coloured as one, and the rotation it is halfway through is still legible from the label beside it.
	text := strings.Join(stages, " ")
	switch {
	case slices.Contains(stages, "AWSCURRENT"):
		return utils.Cell{Text: text, Color: color.FgGreen}
	case slices.Contains(stages, "AWSPENDING"):
		return utils.Cell{Text: text, Color: color.FgYellow}
	default:
		return utils.Cell{Text: text, Color: color.Faint}
	}
}

// secretTime pairs the absolute timestamp with its distance from now: an audit question ("when was this last changed") wants the date, and a rotation question ("is the next one overdue") wants the distance.
func secretTime(t *time.Time, now time.Time) string {
	if t == nil {
		return "none"
	}

	return t.UTC().Format(time.RFC3339) + " (" + RelTime(*t, now) + ")"
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}

	return *t
}

func deref(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// orNone states an absence rather than leaving a blank the reader has to interpret, and it is the only thing that fills an empty AWS field: an overview never invents a value.
func orNone(s string) string {
	if s == "" {
		return "none"
	}

	return s
}
