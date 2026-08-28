package presentation

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// plainRepository renders the overview as the words it chose: escapes stripped and the alignment padding collapsed, since neither is what these states are about.
func plainRepository(r *aws.ECRRepository, images []aws.ECRImage, err error, width int) string {
	return kvPadding.ReplaceAllString(utils.Decolorise(FormatECRRepositoryOverview(r, images, err, width, overviewNow)), " ")
}

func overviewRepository() *aws.ECRRepository {
	created := overviewNow.Add(-90 * 24 * time.Hour)
	evaluated := overviewNow.Add(-6 * time.Hour)

	return &aws.ECRRepository{
		Name:               "app-api",
		Arn:                "arn:aws:ecr:eu-west-1:123456789012:repository/app-api",
		URI:                "123456789012.dkr.ecr.eu-west-1.amazonaws.com/app-api",
		RegistryID:         "123456789012",
		CreatedAt:          &created,
		ScanOnPush:         true,
		TagMutability:      "MUTABLE",
		EncryptionType:     "KMS",
		KMSKey:             "arn:aws:kms:eu-west-1:123456789012:key/2f7c",
		PolicyText:         `{"Version":"2012-10-17"}`,
		LifecyclePolicy:    `{"rules":[]}`,
		LifecycleEvaluated: &evaluated,
	}
}

func overviewImages() []aws.ECRImage {
	newest := overviewNow.Add(-2 * time.Hour)
	older := overviewNow.Add(-30 * 24 * time.Hour)

	return []aws.ECRImage{
		{Digest: "sha256:aaaabbbbccccdddd", Tags: []string{"1.4.0", "latest"}, SizeBytes: 104857600, PushedAt: &newest},
		{Digest: "sha256:eeeeffff00001111", SizeBytes: 52428800, PushedAt: &older},
	}
}

func TestRepositoryOverviewRendersEverySection(t *testing.T) {
	got := plainRepository(overviewRepository(), overviewImages(), nil, stackedWidth)

	for _, want := range []string{
		"Repository", "app-api",
		"Images", "2", "Mutability", "MUTABLE", "Scan on push", "on",
		"123456789012.dkr.ecr.eu-west-1.amazonaws.com/app-api",
		// No scan-on-push row and no header badge: the cards carry both, and the raw enum row stays because MUTABLE_WITH_EXCLUSION and MUTABLE are one card word but two policies.
		"Configuration", "Tag mutability: MUTABLE", "KMS · arn:aws:kms:eu-west-1:123456789012:key/2f7c", "Registry: 123456789012",
		"Policies", "Repository policy: attached, shown on the Config tab", "Lifecycle policy: attached, evaluated 6h ago",
		"Images", "2 images · 150.0 MiB",
		"Tag", "Pushed", "Size", "Digest",
		"1.4.0, latest", "2h ago", "100.0 MiB", "aaaabbbbcccc",
		"(untagged)", "30d ago",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview is missing %q\n%s", want, got)
		}
	}

	plain := utils.Decolorise(FormatECRRepositoryOverview(overviewRepository(), overviewImages(), nil, stackedWidth, overviewNow))
	header := strings.SplitN(plain, "\n\n", 2)[0]
	if strings.Count(header, "┌") != 3 {
		t.Errorf("header does not contain three stat cards\n%s", header)
	}
	if strings.Count(plain, "├") != 1 {
		t.Errorf("overview does not contain one boxed image table\n%s", plain)
	}
}

func TestRepositoryOverviewCardsUsePostureColours(t *testing.T) {
	forceColor(t)

	cards := ecrStatCards(overviewRepository(), overviewImages(), nil)
	want := []Stat{
		{Label: "Images", Value: utils.Cell{Text: "2"}},
		{Label: "Mutability", Value: utils.Cell{Text: "MUTABLE", Color: color.FgYellow}},
		{Label: "Scan on push", Value: utils.Cell{Text: "on", Color: color.FgGreen}},
	}
	if len(cards) != len(want) {
		t.Fatalf("ecrStatCards() returned %d cards, want %d", len(cards), len(want))
	}
	for i := range want {
		if cards[i] != want[i] {
			t.Errorf("card %d = %+v, want %+v", i, cards[i], want[i])
		}
	}

	repo := overviewRepository()
	repo.TagMutability = "IMMUTABLE"
	repo.ScanOnPush = false
	cards = ecrStatCards(repo, nil, nil)
	if got := cards[1].Value; got.Text != "IMMUTABLE" || got.Color != color.FgGreen {
		t.Errorf("immutable card = %+v, want green IMMUTABLE", got)
	}
	if got := cards[2].Value; got.Text != "off" || got.Color != 0 {
		t.Errorf("scan-off card = %+v, want plain off", got)
	}

	rendered := FormatECRRepositoryOverview(overviewRepository(), overviewImages(), nil, stackedWidth, overviewNow)
	if digest := utils.ColoredString("aaaabbbbcccc", color.Faint); !strings.Contains(rendered, digest) {
		t.Errorf("image digest is not faint\n%s", utils.Decolorise(rendered))
	}
}

// The creation stamp carries its age: how old a repository is decides whether an empty one is new or abandoned.
// Asserted on the function and then at a width the header and cards fit in, because narrower panes correctly truncate the URI and age before either can overrun the pane.
func TestRepositoryOverviewDatesTheCreationStamp(t *testing.T) {
	if got, want := ecrCreated(overviewRepository(), overviewNow), "created 2026-05-29T12:00:00Z (90d ago)"; got != want {
		t.Errorf("ecrCreated() = %q, want %q", got, want)
	}

	if got := plainRepository(overviewRepository(), overviewImages(), nil, 180); !strings.Contains(got, "(90d ago)") {
		t.Errorf("overview does not date the creation stamp\n%s", got)
	}
}

// Every optional field a repository can omit says what it is, rather than leaving a blank the reader has to interpret.
func TestRepositoryOverviewStatesEveryAbsence(t *testing.T) {
	got := plainRepository(&aws.ECRRepository{Name: "bare"}, nil, nil, stackedWidth)

	for _, want := range []string{
		// Scan on push lives in its header card, off and plain for a bare repository.
		"Scan on push",
		"off",
		"Encryption: none",
		"Registry: none",
		"ARN: none",
		"Repository policy: none",
		"Lifecycle policy: none",
		"Images\nnone",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview is missing %q\n%s", want, got)
		}
	}
	// A repository with no creation date must not claim one, and RelTime would date a nil to the year 1.
	if strings.Contains(got, "created") {
		t.Errorf("overview invented a creation date\n%s", got)
	}
}

// A policy read that failed is not a repository without a policy, and both leave the field empty.
// Each read is reported on its own row: the two calls fail independently, so the one that answered must keep answering.
func TestRepositoryOverviewTellsAFailedPolicyReadFromAnAbsentPolicy(t *testing.T) {
	absent := plainRepository(&aws.ECRRepository{Name: "bare"}, nil, nil, stackedWidth)
	unreadable := plainRepository(&aws.ECRRepository{
		Name:               "bare",
		PolicyErr:          errors.New("ThrottlingException"),
		LifecyclePolicyErr: errors.New("AccessDenied"),
	}, nil, nil, stackedWidth)

	for _, want := range []string{
		"Repository policy: unavailable: ThrottlingException",
		"Lifecycle policy: unavailable: AccessDenied",
	} {
		if !strings.Contains(unreadable, want) {
			t.Errorf("overview is missing %q\n%s", want, unreadable)
		}
	}
	// Asserting the failure text alone passes while both states still collapse to one string, which is the bug this closes.
	for _, absentLine := range []string{"Repository policy: none", "Lifecycle policy: none"} {
		if strings.Contains(unreadable, absentLine) {
			t.Errorf("an unreadable policy still renders as %q\n%s", absentLine, unreadable)
		}
		if !strings.Contains(absent, absentLine) {
			t.Errorf("a genuinely absent policy no longer renders as %q\n%s", absentLine, absent)
		}
	}

	// One read failing must not take the other's answer with it.
	oneSide := plainRepository(&aws.ECRRepository{Name: "bare", PolicyErr: errors.New("ThrottlingException"), LifecyclePolicy: `{"rules":[]}`}, nil, nil, stackedWidth)
	if !strings.Contains(oneSide, "Lifecycle policy: attached, never evaluated") {
		t.Errorf("a failed repository-policy read took the lifecycle answer down with it\n%s", oneSide)
	}
}

// An attached lifecycle policy that has never run has deleted nothing yet, and reporting it as merely attached hides that.
func TestRepositoryOverviewSeparatesAnUnevaluatedLifecyclePolicy(t *testing.T) {
	repo := overviewRepository()
	repo.LifecycleEvaluated = nil

	if got := plainRepository(repo, nil, nil, stackedWidth); !strings.Contains(got, "Lifecycle policy: attached, never evaluated") {
		t.Errorf("overview does not distinguish an unevaluated lifecycle policy\n%s", got)
	}
}

// A failed DescribeImages costs the image table and nothing else: the repository's own posture came off the list row and is still answerable.
func TestRepositoryOverviewSurvivesAFailedImageFetch(t *testing.T) {
	err := errors.New("AccessDenied")
	got := plainRepository(overviewRepository(), nil, err, stackedWidth)

	if !strings.Contains(got, "Images\nunavailable: AccessDenied") {
		t.Errorf("overview does not report the failed image fetch\n%s", got)
	}
	for _, want := range []string{"app-api", "Tag mutability: MUTABLE", "Repository policy: attached"} {
		if !strings.Contains(got, want) {
			t.Errorf("a failed image fetch took %q down with it\n%s", want, got)
		}
	}
	if count := ecrStatCards(overviewRepository(), nil, err)[0].Value; count.Text != "unavailable" || count.Color != color.FgRed {
		t.Errorf("failed image count card = %+v, want red unavailable", count)
	}
}

// The block claims to show the LATEST images, so it sorts them itself: a fixture that arrives already sorted cannot tell a real ordering from none at all.
func TestRepositoryOverviewOrdersImagesNewestFirst(t *testing.T) {
	oldest := overviewNow.Add(-40 * 24 * time.Hour)
	middle := overviewNow.Add(-5 * 24 * time.Hour)
	newest := overviewNow.Add(-1 * time.Hour)
	images := []aws.ECRImage{
		{Digest: "sha256:oldoldoldold0000", Tags: []string{"oldest"}, PushedAt: &oldest},
		{Digest: "sha256:newnewnewnew0000", Tags: []string{"newest"}, PushedAt: &newest},
		{Digest: "sha256:undatedundated00", Tags: []string{"undated"}},
		{Digest: "sha256:midmidmidmid0000", Tags: []string{"middle"}, PushedAt: &middle},
	}

	got := plainRepository(overviewRepository(), images, nil, stackedWidth)
	order := []string{"newest", "middle", "oldest", "undated"}
	at := make([]int, len(order))
	for i, tag := range order {
		if at[i] = strings.Index(got, tag); at[i] < 0 {
			t.Fatalf("overview is missing the %q image\n%s", tag, got)
		}
	}
	for i := 1; i < len(at); i++ {
		if at[i] < at[i-1] {
			t.Errorf("%q is rendered before %q\n%s", order[i], order[i-1], got)
		}
	}
}

// A repository accumulates images without bound, and silently showing the first few would read as the whole repository.
func TestRepositoryOverviewCapsTheImageTableAndSaysSo(t *testing.T) {
	images := make([]aws.ECRImage, 0, ecrImagesShown+2)
	for i := range ecrImagesShown + 2 {
		pushed := overviewNow.Add(-time.Duration(i) * time.Hour)
		images = append(images, aws.ECRImage{Digest: "sha256:d", Tags: []string{"tag-" + string(rune('a'+i))}, PushedAt: &pushed})
	}

	got := plainRepository(overviewRepository(), images, nil, stackedWidth)
	if want := "12 images"; !strings.Contains(got, want) {
		t.Errorf("overview is missing %q\n%s", want, got)
	}
	if want := "(2 more on the Images tab)"; !strings.Contains(got, want) {
		t.Errorf("overview is missing %q\n%s", want, got)
	}
	// The cap keeps the newest, so the two dropped rows are the two oldest tags.
	if strings.Contains(got, "tag-k") || strings.Contains(got, "tag-l") {
		t.Errorf("overview rendered an image past the cap\n%s", got)
	}
}

func TestRepositoryOverviewImageTableKeepsEveryColumn(t *testing.T) {
	forceColor(t)

	headerColumns := regexp.MustCompile(`Tag\s+Pushed\s+Size\s+Digest`)
	imageColumns := regexp.MustCompile(`1\.4\.0, latest\s+2h ago\s+100\.0 MiB\s+aaaabbbbcccc`)
	for _, width := range []int{80, 110, 120, 160} {
		got := utils.Decolorise(FormatECRRepositoryOverview(overviewRepository(), overviewImages(), nil, width, overviewNow))

		header := lineContaining(got, "Digest")
		if header == "" || !headerColumns.MatchString(header) {
			t.Errorf("at width %d image header lost a column: %q\n%s", width, header, got)
		}
		image := lineContaining(got, "1.4.0")
		if image == "" || !imageColumns.MatchString(image) {
			t.Errorf("at width %d image row lost a column: %q\n%s", width, image, got)
		}
	}
}

// Wrapping is off on an overview, so a line over its budget runs off the pane rather than folding.
func TestRepositoryOverviewNeverExceedsTheWidth(t *testing.T) {
	forceColor(t)

	repo := overviewRepository()
	// A long name in the header, which Columns never measures because it spans the full width, and a tag list that runs past any column.
	repo.Name = "a-very-long-repository-name-nobody-should-have-but-someone-in-eu-west-1-does"
	// A failed policy read puts the SDK's own error text on a kv row, and a real one is longer than anything else this pane holds: measured at 195 cells, it is the only line over budget from width 66 up, so without it the sweep never covers the state this stage added.
	repo.PolicyErr = errors.New("operation error ECR: GetRepositoryPolicy, https response error StatusCode: 400, RequestID: 8f2c1d94-0b7a-4e51-9c3f-2a6d5b8e1f00, ThrottlingException: Rate exceeded")
	images := overviewImages()
	images[0].Tags = []string{"1.4.0", "latest", "release-candidate-2026-08-27-build-1841", "deployed-to-production"}

	for width := 40; width <= 220; width++ {
		for _, line := range strings.Split(FormatECRRepositoryOverview(repo, images, nil, width, overviewNow), "\n") {
			if got := runewidth.StringWidth(utils.Decolorise(line)); got > width {
				t.Fatalf("at width %d a line is %d cells wide: %q", width, got, utils.Decolorise(line))
			}
		}
	}
}
