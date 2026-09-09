package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// fakeECR answers the three calls these tests make and counts them by operation.
type fakeECR struct {
	mu           sync.Mutex
	calls        map[string]int
	repositories int
	policyStatus int // when non-zero, both policy reads answer with this status and policyBody
	policyBody   map[string]any
}

func (f *fakeECR) count(operation string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls[operation]
}

func (f *fakeECR) serve() *httptest.Server {
	f.calls = map[string]int{}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		operation := r.Header.Get("X-Amz-Target")
		if i := strings.LastIndex(operation, "."); i >= 0 {
			operation = operation[i+1:]
		}

		f.mu.Lock()
		f.calls[operation]++
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		if operation == "DescribeRepositories" {
			repos := make([]map[string]any, 0, f.repositories)
			for i := range f.repositories {
				name := fmt.Sprintf("service-%02d", i)
				repos = append(repos, map[string]any{
					"repositoryName": name,
					"repositoryArn":  "arn:aws:ecr:eu-west-1:111111111111:repository/" + name,
					"repositoryUri":  "111111111111.dkr.ecr.eu-west-1.amazonaws.com/" + name,
					"registryId":     "111111111111",
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"repositories": repos})

			return
		}

		status, body := f.policyStatus, f.policyBody
		if status == 0 {
			// ECR reports an unattached policy as an error, and each call has its OWN absence type: answering both with the repository one makes the lifecycle read look like a genuine failure, which is a fixture bug that hides whether the memo works.
			status = http.StatusBadRequest
			absent := "RepositoryPolicyNotFoundException"
			if operation == "GetLifecyclePolicy" {
				absent = "LifecyclePolicyNotFoundException"
			}
			body = map[string]any{"__type": absent, "message": "none"}
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func (f *fakeECR) client(t *testing.T) *Client {
	t.Helper()

	server := f.serve()
	t.Cleanup(server.Close)

	return newClientFromConfig(aws.Config{
		Region:       "eu-west-1",
		Credentials:  credentials.NewStaticCredentialsProvider("ecr-test", "ecr-test", ""),
		BaseEndpoint: aws.String(server.URL),
	})
}

// Listing a registry must cost one call per page and nothing per repository.
// It used to fetch both policy documents for every row, which made a list of thirty repositories 61 sequential calls sharing one deadline: the panel then failed with "context deadline exceeded" attributed to DescribeRepositories, and nothing in the list rows ever read what those 60 calls fetched.
func TestListingRepositoriesCostsOneCallPerPage(t *testing.T) {
	fake := &fakeECR{repositories: 30}
	client := fake.client(t)

	repos, err := client.ListECRRepositoriesDetailed(context.Background())
	if err != nil {
		t.Fatalf("ListECRRepositoriesDetailed() = %v", err)
	}
	if len(repos) != 30 {
		t.Fatalf("listed %d repositories, want 30", len(repos))
	}

	if got := fake.count("DescribeRepositories"); got != 1 {
		t.Errorf("DescribeRepositories called %d times, want 1", got)
	}
	for _, operation := range []string{"GetRepositoryPolicy", "GetLifecyclePolicy"} {
		if got := fake.count(operation); got != 0 {
			t.Errorf("listing %d repositories called %s %d times; the list must not fan out per repository", len(repos), operation, got)
		}
	}
}

// The two policy reads happen for the repository on screen, and the memo is what stops a pane that redraws on a timer from re-asking for a document that changes on a deploy.
func TestRepositoryPoliciesCostTwoCallsAndAreMemoised(t *testing.T) {
	fake := &fakeECR{repositories: 1}
	client := fake.client(t)

	for range 5 {
		if _, err := client.GetECRRepositoryPolicies(context.Background(), "service-00", time.Hour); err != nil {
			t.Fatalf("GetECRRepositoryPolicies() = %v", err)
		}
	}

	for _, operation := range []string{"GetRepositoryPolicy", "GetLifecyclePolicy"} {
		if got := fake.count(operation); got != 1 {
			t.Errorf("%s called %d times across five reads of the same repository, want 1", operation, got)
		}
	}

	// A different repository is a different key, so it pays its own two calls rather than inheriting the answer.
	if _, err := client.GetECRRepositoryPolicies(context.Background(), "service-01", time.Hour); err != nil {
		t.Fatalf("GetECRRepositoryPolicies() for a second repository = %v", err)
	}
	if got := fake.count("GetRepositoryPolicy"); got != 2 {
		t.Errorf("GetRepositoryPolicy called %d times for two repositories, want 2", got)
	}
}

// A failed read must not be memoised: pinning "unavailable" on the pane for the whole staleness window turns one rate-limit or one blip into a minute of blank fields, and the next redraw is the natural retry.
func TestAFailedPolicyReadIsNotMemoised(t *testing.T) {
	// AccessDenied rather than a throttle: the adaptive retryer answers a throttle with backoff, which is correct behaviour and twenty seconds of it in a unit test. What is under test is the caching decision, and a denial exercises it without the wait.
	fake := &fakeECR{
		repositories: 1,
		policyStatus: http.StatusForbidden,
		policyBody:   map[string]any{"__type": "AccessDeniedException", "message": "denied"},
	}
	client := fake.client(t)

	first, err := client.GetECRRepositoryPolicies(context.Background(), "service-00", time.Hour)
	if err != nil {
		t.Fatalf("GetECRRepositoryPolicies() = %v", err)
	}
	if first.PolicyErr == nil {
		t.Fatal("a denied policy read reported no error, so the pane would state an absence it cannot know")
	}

	if _, err := client.GetECRRepositoryPolicies(context.Background(), "service-00", time.Hour); err != nil {
		t.Fatalf("second GetECRRepositoryPolicies() = %v", err)
	}
	if got := fake.count("GetRepositoryPolicy"); got < 2 {
		t.Errorf("GetRepositoryPolicy called %d times, want the failed read attempted again rather than cached", got)
	}
}

func TestGetECRRepositoryPoliciesGuards(t *testing.T) {
	if _, err := (&Client{}).GetECRRepositoryPolicies(context.Background(), "repo", 0); err == nil {
		t.Error("GetECRRepositoryPolicies() with no ECR client should error")
	}

	fake := &fakeECR{repositories: 1}
	if _, err := fake.client(t).GetECRRepositoryPolicies(context.Background(), "", 0); err == nil {
		t.Error("GetECRRepositoryPolicies() with no repository name should error")
	}
	if fake.count("GetRepositoryPolicy") != 0 {
		t.Error("a call with no repository name still reached ECR")
	}
}

// The guard that keeps a read-only session read-only must still let these through: they are reads, and refusing them would leave the Policies tab permanently empty.
func TestThePolicyReadsAreAllowedWhileWritesAreDenied(t *testing.T) {
	fake := &fakeECR{repositories: 1}
	client := fake.client(t)

	policies, err := client.GetECRRepositoryPolicies(context.Background(), "service-00", 0)
	if err != nil {
		t.Fatalf("GetECRRepositoryPolicies() = %v", err)
	}
	for _, err := range policies.Errs() {
		if errors.Is(err, ErrReadOnly) {
			t.Errorf("a policy read was refused as a write: %v", err)
		}
	}
}
