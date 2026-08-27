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

// ECRRepositoryWeights gives the repository name the slack; both policy columns are fixed words.
func ECRRepositoryWeights() []int {
	return []int{1, 0, 0}
}

func GetECRRepositoryDisplayCells(r *aws.ECRRepository) []utils.Cell {
	scan := "scan off"
	if r.ScanOnPush {
		scan = "scan on"
	}

	return []utils.Cell{
		{Text: r.Name, Color: color.Bold},
		ecrMutabilityBadge(r.TagMutability),
		{Text: scan},
	}
}

// ecrMutabilityBadge colours only the mutable case: a mutable tag can be overwritten under a running deployment, so the digest behind "latest" is not fixed, while an immutable repository is the posture nobody needs alerting to.
// ECR reports four values, so the match is on the prefix; the two _WITH_EXCLUSION variants carry an exclusion list that makes the policy partial, and are starred rather than rendered as a blanket policy they are not. The raw value is on the repository's Config tab.
func ecrMutabilityBadge(mutability string) utils.Cell {
	star := ""
	if strings.HasSuffix(mutability, "_WITH_EXCLUSION") {
		star = "*"
	}

	switch {
	case strings.HasPrefix(mutability, "IMMUTABLE"):
		return utils.Cell{Text: "immutable" + star}
	case strings.HasPrefix(mutability, "MUTABLE"):
		return utils.Cell{Text: "● mutable" + star, Color: color.FgYellow}
	case mutability == "":
		return utils.Cell{Text: "-"}
	default:
		// An enum value this build does not know about is shown as AWS sent it rather than guessed at.
		return utils.Cell{Text: mutability}
	}
}

// ShortDigest follows Docker's convention to keep identity recognizable.
func ShortDigest(digest string) string {
	d := strings.TrimPrefix(digest, "sha256:")
	if len(d) > 12 {
		return d[:12]
	}

	return d
}

// ecrImagesShown caps the image table. A repository accumulates images without bound and the overview answers "what is in here now", not "what has ever been pushed".
const ecrImagesShown = 10

// FormatECRRepositoryOverview lays a repository out for the Overview tab: the posture that decides whether a tag can move under a deployment, then what is actually in the repository.
// Everything but the image list is already on the row the list fetched, so this pane costs the one DescribeImages call the Images tab makes.
func FormatECRRepositoryOverview(r *aws.ECRRepository, images []aws.ECRImage, imagesErr error, width int, now time.Time) string {
	// Cut to the pane: the header spans the full width rather than a column, so Columns never measures it, and a registry URI plus a long repository name runs off the edge unmarked.
	header := truncateBlock(ResourceHeader("Repository", r.Name, ecrMutabilityBadge(r.TagMutability).Rendered(), "", r.URI, ecrCreated(r, now)), width)

	column := ColumnWidth(width, overviewGap)
	left := joinBlocks(ecrConfigBlock(r), ecrPolicyBlock(r, now))
	right := ecrImagesBlock(images, imagesErr, column, now)

	return header + "\n\n" + Columns(width, overviewGap, left, right)
}

func ecrCreated(r *aws.ECRRepository, now time.Time) string {
	if r.CreatedAt == nil {
		return ""
	}

	return "created " + r.CreatedAt.UTC().Format(time.RFC3339) + " (" + RelTime(*r.CreatedAt, now) + ")"
}

func ecrConfigBlock(r *aws.ECRRepository) string {
	rows := []kv{
		// The raw enum rather than the header's badge word: MUTABLE_WITH_EXCLUSION and MUTABLE are one badge but two policies, and this is the row an audit reads.
		{"Tag mutability", orNone(r.TagMutability)},
		{"Scan on push", ecrScanLine(r.ScanOnPush)},
		{"Encryption", ecrEncryptionLine(r)},
		{"Registry", orNone(r.RegistryID)},
		{"ARN", orNone(r.Arn)},
	}

	return SectionTitle("Configuration") + "\n" + kvBlock(rows)
}

// ecrScanLine colours the off case: basic scanning is free and per-push, so a repository with it off is a deliberate gap rather than a cost decision.
func ecrScanLine(on bool) string {
	if on {
		return "on"
	}

	return utils.ColoredString("off", color.FgYellow)
}

// ecrEncryptionLine names the key, not just the fact: AES256 and KMS differ in who can decrypt a layer, which is the question asked of a repository holding production images.
func ecrEncryptionLine(r *aws.ECRRepository) string {
	if r.EncryptionType == "" {
		return "none"
	}
	if r.KMSKey != "" {
		return r.EncryptionType + " · " + r.KMSKey
	}

	return r.EncryptionType
}

// ecrPolicyBlock reports each policy against its OWN read: the two calls fail independently, so one unavailable section would delete the answer the other one returned.
func ecrPolicyBlock(r *aws.ECRRepository, now time.Time) string {
	rows := []kv{
		{"Repository policy", ecrPolicyLine(r)},
		{"Lifecycle policy", ecrLifecycleLine(r, now)},
	}

	return SectionTitle("Policies") + "\n" + kvBlock(rows)
}

// ecrPolicyLine keeps a policy that could not be read apart from a repository that has none.
// Both are the empty string, and the list fetch spends one deadline on the repository pages plus two policy calls per repository, so a timeout, a throttle or a denial reaching here is not evidence of an absence and "none" would be the pane inventing the safer of the two answers.
func ecrPolicyLine(r *aws.ECRRepository) string {
	if r.PolicyErr != nil {
		return fieldUnavailable(r.PolicyErr)
	}
	if r.PolicyText != "" {
		return "attached, shown on the Config tab"
	}

	return "none"
}

// ecrLifecycleLine carries the last evaluation because an attached lifecycle policy that has never run has not deleted anything yet, and the two states look identical without it.
// A read that failed is a third state: the stamp is nil then too, so it is checked before either.
func ecrLifecycleLine(r *aws.ECRRepository, now time.Time) string {
	if r.LifecyclePolicyErr != nil {
		return fieldUnavailable(r.LifecyclePolicyErr)
	}
	if r.LifecyclePolicy == "" {
		return "none"
	}
	if r.LifecycleEvaluated == nil {
		return "attached, never evaluated"
	}

	return "attached, evaluated " + RelTime(*r.LifecycleEvaluated, now)
}

func ecrImagesBlock(images []aws.ECRImage, err error, width int, now time.Time) string {
	title := SectionTitle("Images")
	if err != nil {
		return title + "\n" + utils.ColoredString("unavailable: "+err.Error(), color.FgRed)
	}
	if len(images) == 0 {
		return title + "\nnone"
	}

	var total int64
	for _, image := range images {
		total += image.SizeBytes
	}
	summary := fmt.Sprintf("%s · %s", pluralize(len(images), "image"), FormatByteCount(float64(total)))

	// Sorted here rather than trusted from the caller: this block claims to show the LATEST images, and that claim cannot rest on which fetch happened to fill the slice.
	newest := slices.Clone(images)
	slices.SortStableFunc(newest, byNewestPush)
	shown := newest
	if len(shown) > ecrImagesShown {
		shown = shown[:ecrImagesShown]
	}

	rows := make([][]utils.Cell, len(shown))
	for i, image := range shown {
		rows[i] = []utils.Cell{
			ecrTagsCell(image.Tags),
			{Text: ShortDigest(image.Digest), Color: color.Faint},
			{Text: FormatByteCount(float64(image.SizeBytes))},
			{Text: RelTime(pushedAt(image.PushedAt), now), Color: color.Faint},
			ecrFindingsCell(image.Severity),
		}
	}

	// The tag column is the only one without a natural width, so it takes the slack and every other column is content-sized.
	table, _ := utils.RenderTableFit(rows, width, []int{1, 0, 0, 0, 0})

	out := title + "\n" + summary + "\n" + table
	if hidden := len(images) - len(shown); hidden > 0 {
		out += "\n" + utils.ColoredString(fmt.Sprintf("(%d more on the Images tab)", hidden), color.Faint)
	}

	return out
}

// byNewestPush orders images newest first, with undated ones last, matching how the fetch orders them.
// An image manifest can carry no push time, and treating that as the zero time would float it to the top of a list whose whole point is recency.
func byNewestPush(a, b aws.ECRImage) int {
	switch {
	case a.PushedAt == nil && b.PushedAt == nil:
		return 0
	case a.PushedAt == nil:
		return 1
	case b.PushedAt == nil:
		return -1
	}

	return b.PushedAt.Compare(*a.PushedAt)
}

func pushedAt(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}

	return *t
}

// ecrTagsCell renders an image's tags. An image with none is untagged rather than blank: an untagged image is usually a leftover a lifecycle rule should have removed.
func ecrTagsCell(tags []string) utils.Cell {
	if len(tags) == 0 {
		return utils.Cell{Text: "(untagged)", Color: color.Faint}
	}

	return utils.Cell{Text: strings.Join(tags, ", ")}
}

// ecrSeverityRank orders the finding severities ECR reports, worst first, so a row shows the one that decides whether the image ships.
var ecrSeverityRank = []string{"CRITICAL", "HIGH", "MEDIUM", "LOW", "INFORMATIONAL", "UNDEFINED"}

// ecrFindingsCell reduces a scan summary to its worst severity and that severity's count.
// A repository with scanning off reports no counts at all, which is not the same as a clean scan, so an absent summary is "-" rather than "0".
func ecrFindingsCell(severity map[string]int32) utils.Cell {
	if len(severity) == 0 {
		return utils.Cell{Text: "-", Color: color.Faint}
	}

	for _, name := range ecrSeverityRank {
		count, ok := severity[name]
		if !ok || count == 0 {
			continue
		}

		text := fmt.Sprintf("%s %d", strings.ToLower(name), count)
		switch name {
		case "CRITICAL", "HIGH":
			return utils.Cell{Text: text, Color: color.FgRed}
		case "MEDIUM":
			return utils.Cell{Text: text, Color: color.FgYellow}
		default:
			return utils.Cell{Text: text, Color: color.Faint}
		}
	}

	// Every severity ECR reported was zero, which is a scan that ran and found nothing.
	return utils.Cell{Text: "clean", Color: color.FgGreen}
}
