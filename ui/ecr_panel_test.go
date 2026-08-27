package ui

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/noelruault/lazyaws/apps/aws"
)

func TestFormatECRConfigNoLifecycle(t *testing.T) {
	repo := &aws.ECRRepository{Name: "svc-api", URI: "123.dkr.ecr.us-east-1.amazonaws.com/svc-api", TagMutability: "MUTABLE"}
	out := formatECRConfig(repo)
	if !strings.Contains(out, "svc-api") || !strings.Contains(out, repo.URI) {
		t.Errorf("expected name and URI, got:\n%s", out)
	}
	if !strings.Contains(out, "Lifecycle Policy:\nnot configured") {
		t.Errorf("expected 'not configured' lifecycle, got:\n%s", out)
	}
}

// The Config tab publishes the same two policy fields the Overview does, so it has the same two states to keep apart: "not configured" is a claim only a successful read supports.
func TestFormatECRConfigTellsAFailedPolicyReadFromAnAbsentPolicy(t *testing.T) {
	out := formatECRConfig(&aws.ECRRepository{
		Name:               "svc-api",
		PolicyErr:          errors.New("ThrottlingException"),
		LifecyclePolicyErr: errors.New("AccessDenied"),
	})

	for _, want := range []string{"Repository Policy:\nunavailable: ThrottlingException", "Lifecycle Policy:\nunavailable: AccessDenied"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "not configured") {
		t.Errorf("an unreadable policy still reads as not configured:\n%s", out)
	}
}

// Every policy read the list fetch makes has to reach the backoff engine, or the pane keeps asking at the rate that earned the throttle.
// The two policy calls run per repository inside the list fetch and never surface as its error, so the Overview's own error is not enough.
func TestECROverviewErrsCarriesEveryThrottleableRead(t *testing.T) {
	imagesErr := errors.New("DescribeImages")
	policyErr := errors.New("GetRepositoryPolicy")
	lifecycleErr := errors.New("GetLifecyclePolicy")

	got := ecrOverviewErrs(&aws.ECRRepository{PolicyErr: policyErr, LifecyclePolicyErr: lifecycleErr}, imagesErr)
	for _, want := range []error{imagesErr, policyErr, lifecycleErr} {
		if !slices.ContainsFunc(got, func(err error) bool { return errors.Is(err, want) }) {
			t.Errorf("ecrOverviewErrs() does not carry %v, got %v", want, got)
		}
	}
}

func TestFormatECRConfigWithLifecycle(t *testing.T) {
	repo := &aws.ECRRepository{Name: "svc-api", LifecyclePolicy: `{"rules":[]}`}
	out := formatECRConfig(repo)
	if !strings.Contains(out, `{"rules":[]}`) {
		t.Errorf("expected lifecycle policy text, got:\n%s", out)
	}
}

func TestFormatECREncryption(t *testing.T) {
	if got := formatECREncryption(&aws.ECRRepository{}); got != "-" {
		t.Errorf("expected '-' for no encryption, got %q", got)
	}
	if got := formatECREncryption(&aws.ECRRepository{EncryptionType: "KMS", KMSKey: "arn:aws:kms:1"}); got != "KMS (arn:aws:kms:1)" {
		t.Errorf("expected KMS with key, got %q", got)
	}
	if got := formatECREncryption(&aws.ECRRepository{EncryptionType: "AES256"}); got != "AES256" {
		t.Errorf("expected bare algorithm, got %q", got)
	}
}

func TestFormatECRImagesEmpty(t *testing.T) {
	if got := formatECRImages(nil); got != "no images\n" {
		t.Errorf("expected 'no images', got %q", got)
	}
}

func TestFormatECRImagesUntagged(t *testing.T) {
	out := formatECRImages([]aws.ECRImage{{Digest: "sha256:abcdef0123456789", SizeBytes: 1024}})
	if !strings.Contains(out, "(untagged)") {
		t.Errorf("expected untagged marker, got:\n%s", out)
	}
	if !strings.Contains(out, "abcdef012345") {
		t.Errorf("expected short digest, got:\n%s", out)
	}
}

func TestShortDigest(t *testing.T) {
	if got := shortDigest("sha256:abcdef0123456789"); got != "abcdef012345" {
		t.Errorf("shortDigest() = %q, want 12-char hex", got)
	}
	if got := shortDigest("short"); got != "short" {
		t.Errorf("shortDigest() with short input = %q, want unchanged", got)
	}
}

func TestFirstTaggedImageDigest(t *testing.T) {
	images := []aws.ECRImage{
		{Digest: "sha256:untagged"},
		{Digest: "sha256:tagged", Tags: []string{"v1.2.3"}},
	}
	if got := firstTaggedImageDigest(images); got != "sha256:tagged" {
		t.Errorf("firstTaggedImageDigest() = %q, want the first tagged image", got)
	}
	if got := firstTaggedImageDigest([]aws.ECRImage{{Digest: "sha256:untagged"}}); got != "" {
		t.Errorf("firstTaggedImageDigest() with no tags = %q, want empty", got)
	}
}

func TestFormatECRScanEnhancedFindingsPreferred(t *testing.T) {
	scan := &aws.ECRScanResult{
		Status: "COMPLETE",
		Findings: []aws.ECRScanFinding{
			{Name: "CVE-legacy", Severity: "HIGH"},
		},
		EnhancedFindings: []aws.ECREnhancedFinding{
			{Title: "CVE-2024-1234", Severity: "CRITICAL", CVSSScore: 9.8, FixAvailable: "YES", VulnerablePackages: []string{"openssl (usr/lib/openssl)"}},
		},
	}
	out := formatECRScan(scan)
	if !strings.Contains(out, "CVE-2024-1234") || !strings.Contains(out, "9.8") {
		t.Errorf("expected enhanced finding with CVSS score, got:\n%s", out)
	}
	if !strings.Contains(out, "Fixable: yes") {
		t.Errorf("expected fixable=yes, got:\n%s", out)
	}
	if strings.Contains(out, "CVE-legacy") {
		t.Errorf("expected legacy findings NOT shown when enhanced findings exist, got:\n%s", out)
	}
}

func TestFormatECRScanLegacyFallback(t *testing.T) {
	scan := &aws.ECRScanResult{
		Status:   "COMPLETE",
		Findings: []aws.ECRScanFinding{{Name: "CVE-legacy", Severity: "HIGH", URI: "https://example.com/cve"}},
	}
	out := formatECRScan(scan)
	if !strings.Contains(out, "CVE-legacy") || !strings.Contains(out, "https://example.com/cve") {
		t.Errorf("expected legacy finding fallback, got:\n%s", out)
	}
}

func TestFormatECRScanNoFindings(t *testing.T) {
	out := formatECRScan(&aws.ECRScanResult{Status: "COMPLETE"})
	if !strings.Contains(out, "no findings") {
		t.Errorf("expected 'no findings', got:\n%s", out)
	}
}

func TestFixableLabel(t *testing.T) {
	cases := map[string]string{"YES": "yes", "PARTIAL": "partially", "NO": "no", "": "unknown"}
	for in, want := range cases {
		if got := fixableLabel(in); got != want {
			t.Errorf("fixableLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatECRConfigCreatedDate(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	out := formatECRConfig(&aws.ECRRepository{Name: "x", CreatedAt: &created})
	if !strings.Contains(out, "2026-01-02") {
		t.Errorf("expected formatted created date, got:\n%s", out)
	}
}
