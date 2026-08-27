package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrTypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
)

// The empty policy is both states, so a read that failed has to leave the error behind: dropping it is what made the Overview and the Config tab call an unreadable policy an absent one, and the drop is invisible from the field alone.
// ECR is not Secrets Manager here: it reports an unattached policy AS RepositoryPolicyNotFoundException, so treating every error as a failed read would state "unavailable" on every repository that simply has no policy.
func TestRepositoryPolicyResultTellsAFailedReadFromAnAbsentPolicy(t *testing.T) {
	readErr := errors.New("ThrottlingException")

	for _, tt := range []struct {
		name    string
		out     *ecr.GetRepositoryPolicyOutput
		err     error
		want    string
		wantErr error
	}{
		{name: "the read failed", err: readErr, wantErr: readErr},
		{name: "a policy that failed to read is never kept", out: &ecr.GetRepositoryPolicyOutput{PolicyText: aws.String(`{"Version":"2012-10-17"}`)}, err: readErr, wantErr: readErr},
		{name: "no policy attached is an error, not an empty body", err: &ecrTypes.RepositoryPolicyNotFoundException{}},
		{name: "a policy is attached", out: &ecr.GetRepositoryPolicyOutput{PolicyText: aws.String(`{"Version":"2012-10-17"}`)}, want: `{"Version":"2012-10-17"}`},
	} {
		policy, err := repositoryPolicyResult(tt.out, tt.err)
		if policy != tt.want || !errors.Is(err, tt.wantErr) {
			t.Errorf("repositoryPolicyResult(%s) = (%q, %v), want (%q, %v)", tt.name, policy, err, tt.want, tt.wantErr)
		}
	}
}

// The last evaluation goes with the policy: a nil stamp reads as "attached, never evaluated", which is a claim only a successful read can support.
func TestLifecyclePolicyResultTellsAFailedReadFromAnAbsentPolicy(t *testing.T) {
	readErr := errors.New("ThrottlingException")
	evaluated := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		name          string
		out           *ecr.GetLifecyclePolicyOutput
		err           error
		want          string
		wantEvaluated *time.Time
		wantErr       error
	}{
		{name: "the read failed", err: readErr, wantErr: readErr},
		{name: "a policy that failed to read is never kept", out: &ecr.GetLifecyclePolicyOutput{LifecyclePolicyText: aws.String(`{"rules":[]}`), LastEvaluatedAt: &evaluated}, err: readErr, wantErr: readErr},
		{name: "no policy set is an error, not an empty body", err: &ecrTypes.LifecyclePolicyNotFoundException{}},
		{name: "a policy is attached but has never run", out: &ecr.GetLifecyclePolicyOutput{LifecyclePolicyText: aws.String(`{"rules":[]}`)}, want: `{"rules":[]}`},
		{name: "a policy is attached and has run", out: &ecr.GetLifecyclePolicyOutput{LifecyclePolicyText: aws.String(`{"rules":[]}`), LastEvaluatedAt: &evaluated}, want: `{"rules":[]}`, wantEvaluated: &evaluated},
	} {
		policy, at, err := lifecyclePolicyResult(tt.out, tt.err)
		if policy != tt.want || at != tt.wantEvaluated || !errors.Is(err, tt.wantErr) {
			t.Errorf("lifecyclePolicyResult(%s) = (%q, %v, %v), want (%q, %v, %v)", tt.name, policy, at, err, tt.want, tt.wantEvaluated, tt.wantErr)
		}
	}
}

func TestPreviewLifecyclePolicyGuards(t *testing.T) {
	if _, err := (&Client{}).PreviewLifecyclePolicy(context.Background(), "repo", ""); err == nil {
		t.Error("PreviewLifecyclePolicy() with nil ECR client should error")
	}
}

func TestPutECRLifecyclePolicyGuards(t *testing.T) {
	if err := (&Client{}).PutECRLifecyclePolicy(context.Background(), "repo", "{}"); err == nil {
		t.Error("PutECRLifecyclePolicy() with nil ECR client should error")
	}
}

func TestDeleteECRRepositoryGuards(t *testing.T) {
	if err := (&Client{}).DeleteECRRepository(context.Background(), "repo", false); err == nil {
		t.Error("DeleteECRRepository() with nil ECR client should error")
	}
}

func TestDeleteECRImageGuards(t *testing.T) {
	if err := (&Client{}).DeleteECRImage(context.Background(), "repo", "sha256:abc"); err == nil {
		t.Error("DeleteECRImage() with nil ECR client should error")
	}
}
