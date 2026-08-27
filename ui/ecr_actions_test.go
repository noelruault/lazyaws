package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
)

// The preview refuses on an empty policy, which is also what a failed read leaves behind: told a repository has no lifecycle policy, an operator writes one over the policy that is already there.
// The guard runs before any client call, so the nil Gui is never reached.
func TestPreviewLifecyclePolicyTellsAFailedReadFromAnAbsentPolicy(t *testing.T) {
	readErr := errors.New("ThrottlingException")
	gui := &Gui{}

	err := gui.ecrPreviewLifecyclePolicy(&aws.ECRRepository{Name: "svc-api", LifecyclePolicyErr: readErr})(context.Background(), "")
	if !errors.Is(err, readErr) {
		t.Errorf("preview on an unreadable policy = %v, want it to carry %v", err, readErr)
	}
	if err != nil && strings.Contains(err.Error(), "has no lifecycle policy") {
		t.Errorf("preview reports an unreadable policy as an absent one: %v", err)
	}

	absent := gui.ecrPreviewLifecyclePolicy(&aws.ECRRepository{Name: "svc-api"})(context.Background(), "")
	if absent == nil || !strings.Contains(absent.Error(), "has no lifecycle policy to preview") {
		t.Errorf("preview on a repository with no policy = %v, want it to say so", absent)
	}
}

func TestFormatECRLifecyclePolicyPreviewNoExpiring(t *testing.T) {
	preview := &aws.ECRLifecyclePolicyPreview{Status: "COMPLETE", ExpiringImageCount: 0}
	out := formatECRLifecyclePolicyPreview(preview)
	if !strings.Contains(out, "COMPLETE") || !strings.Contains(out, "0") {
		t.Errorf("expected status and zero count, got:\n%s", out)
	}
}

func TestFormatECRLifecyclePolicyPreviewWithExpiring(t *testing.T) {
	preview := &aws.ECRLifecyclePolicyPreview{
		Status:             "COMPLETE",
		ExpiringImageCount: 2,
		ExpiringImages: []aws.ECRLifecyclePolicyPreviewImage{
			{Tags: []string{"v1"}, Digest: "sha256:abcdef012345678"},
			{Digest: "sha256:987654321000000"},
		},
	}
	out := formatECRLifecyclePolicyPreview(preview)
	if !strings.Contains(out, "v1") {
		t.Errorf("expected tagged image tag, got:\n%s", out)
	}
	if !strings.Contains(out, "(untagged)") {
		t.Errorf("expected untagged marker for second image, got:\n%s", out)
	}
}

func TestEcrImageLabel(t *testing.T) {
	if got := ecrImageLabel(aws.ECRImage{Digest: "sha256:abcdef012345678", Tags: []string{"v1", "latest"}}); got != "v1, latest (abcdef012345)" {
		t.Errorf("ecrImageLabel() = %q", got)
	}
	if got := ecrImageLabel(aws.ECRImage{Digest: "sha256:abcdef012345678"}); got != "(untagged) (abcdef012345)" {
		t.Errorf("ecrImageLabel() untagged = %q", got)
	}
}

func TestEcrToggleLabelSaysWhichWayItGoes(t *testing.T) {
	if got := ecrToggleLabel(true, "scan-on-push"); got != "Enable scan-on-push" {
		t.Errorf("ecrToggleLabel(true) = %q", got)
	}
	if got := ecrToggleLabel(false, "scan-on-push"); got != "Disable scan-on-push" {
		t.Errorf("ecrToggleLabel(false) = %q", got)
	}
}
