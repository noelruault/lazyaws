package aws

import (
	"context"
	"testing"
)

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
