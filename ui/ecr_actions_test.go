package ui

import (
	"strings"
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
)

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
