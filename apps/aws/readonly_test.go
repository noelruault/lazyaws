package aws

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// permitWrites opens the gate for one test and closes it again, because the gate is process wide and a test that leaves it open would grant writes to every test after it.
func permitWrites(t *testing.T) {
	t.Helper()
	writesAllowed.Store(true)
	t.Cleanup(func() { writesAllowed.Store(false) })
}

// countingEndpoint stands in for AWS and counts what actually arrives, which is how a refusal is told apart from a request that failed for some other reason.
func countingEndpoint(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	var requests atomic.Int64
	server := httptest.NewServer(nil)
	server.Config.Handler = handlerFunc(func() { requests.Add(1) })
	t.Cleanup(server.Close)

	return server, &requests
}

func ec2ClientAgainst(t *testing.T, endpoint string) *ec2.Client {
	t.Helper()

	return newClientFromConfig(aws.Config{
		Region:       "eu-west-1",
		Credentials:  credentials.NewStaticCredentialsProvider("guard-test", "guard-test", ""),
		BaseEndpoint: aws.String(endpoint),
	}).EC2
}

// The promise a read-only session makes is not that the UI hides the buttons; it is that the call is never made.
// So this asserts the refusal AND that nothing reached the endpoint: a guard that let the request out and ignored the answer would pass the first half alone.
func TestAMutatingCallIsRefusedBeforeItReachesAWS(t *testing.T) {
	server, requests := countingEndpoint(t)
	client := ec2ClientAgainst(t, server.URL)

	_, err := client.TerminateInstances(context.Background(), &ec2.TerminateInstancesInput{
		InstanceIds: []string{"i-0123456789abcdef0"},
	})
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("TerminateInstances error = %v, want it to unwrap to ErrReadOnly", err)
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("the refused call still sent %d request(s) to AWS", got)
	}
	// The message has to name the operation: "something was refused" sends a user hunting through eight panels for what it was.
	if !strings.Contains(err.Error(), "TerminateInstances") {
		t.Errorf("refusal does not name the operation: %v", err)
	}
}

// The other half of the promise: reads are not collateral damage. A guard that refused everything would also pass the test above.
func TestAReadIsNotRefusedWhileWritesAreDenied(t *testing.T) {
	server, requests := countingEndpoint(t)
	client := ec2ClientAgainst(t, server.URL)

	// The endpoint answers with an empty body, so the call fails to parse. What matters is that it was sent and not refused.
	_, err := client.DescribeInstances(context.Background(), &ec2.DescribeInstancesInput{})
	if errors.Is(err, ErrReadOnly) {
		t.Fatalf("DescribeInstances was refused as a write: %v", err)
	}
	if got := requests.Load(); got == 0 {
		t.Error("DescribeInstances never reached the endpoint, so reads are being blocked too")
	}
}

func TestTheFlagIsWhatLetsAMutatingCallThrough(t *testing.T) {
	permitWrites(t)

	server, requests := countingEndpoint(t)
	client := ec2ClientAgainst(t, server.URL)

	_, err := client.TerminateInstances(context.Background(), &ec2.TerminateInstancesInput{
		InstanceIds: []string{"i-0123456789abcdef0"},
	})
	if errors.Is(err, ErrReadOnly) {
		t.Fatalf("TerminateInstances was refused even with writes allowed: %v", err)
	}
	if got := requests.Load(); got == 0 {
		t.Error("TerminateInstances never reached the endpoint with writes allowed")
	}
}

// A client built before the flag was ever read must still answer to the gate, because the profile switch and the cached-credentials path both build clients at moments the startup sequence does not control.
func TestThePolicyIsReadPerCallRatherThanBakedIntoTheClient(t *testing.T) {
	server, _ := countingEndpoint(t)
	client := ec2ClientAgainst(t, server.URL)

	permitWrites(t)
	if _, err := client.TerminateInstances(context.Background(), &ec2.TerminateInstancesInput{InstanceIds: []string{"i-0"}}); errors.Is(err, ErrReadOnly) {
		t.Fatalf("a client built while writes were denied stayed denied after the flag: %v", err)
	}

	writesAllowed.Store(false)
	if _, err := client.TerminateInstances(context.Background(), &ec2.TerminateInstancesInput{InstanceIds: []string{"i-0"}}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("the same client kept its write permission after the gate closed: %v", err)
	}
}

func TestReadOperationClassifiesTheEdges(t *testing.T) {
	for _, tc := range []struct {
		operation string
		read      bool
		why       string
	}{
		{"DescribeInstances", true, "a plain read verb"},
		{"GetSecretValue", true, "revealing a secret reads it"},
		{"ListObjectsV2", true, "the version suffix must not defeat the prefix match"},
		{"HeadObject", true, "a metadata read"},
		{"FilterLogEvents", true, "log search"},
		{"AssumeRole", true, "credential resolution for a profile with role_arn"},
		{"CreateToken", true, "the SSO token refresh, without which the session cannot even read"},
		{"ConverseStream", true, "model inference answers and creates nothing"},
		{"StartLiveTail", true, "a log tail AWS happened to name Start"},

		{"TerminateInstances", false, "the one that matters"},
		{"StopInstances", false, ""},
		{"UpdateService", false, ""},
		{"DeleteObject", false, ""},
		{"PutBucketVersioning", false, ""},
		{"SendSSHPublicKey", false, "pushes a key to an instance"},
		{"ModifyInstanceAttribute", false, ""},
		{"AbortMultipartUpload", false, "aborting is still a change"},
		{"StartLifecyclePolicyPreview", false, "a dry run is still a call that asks ECR to create something"},
		{"RestoreSecret", false, ""},
		{"", false, "an unnamed operation must fail closed rather than be waved through"},
	} {
		if got := readOperation(tc.operation); got != tc.read {
			t.Errorf("readOperation(%q) = %v, want %v (%s)", tc.operation, got, tc.read, tc.why)
		}
	}
}

// readOnlyOperations and mutatingOperations are every AWS operation this package calls, classified once, on purpose.
// The test below reads the calls back out of the source, so adding one to the code without adding it here fails: the point is that nobody can introduce an operation without a reviewer deciding which side of the gate it belongs on.
var readOnlyOperations = []string{
	"DescribeAccessEntry", "DescribeAddon", "DescribeAddresses", "DescribeAlarms", "DescribeAutoScalingGroups",
	"DescribeAutoScalingInstances", "DescribeCluster", "DescribeClusters", "DescribeContainerInstances",
	"DescribeFargateProfile", "DescribeImageScanFindings", "DescribeImages", "DescribeInstanceAttribute",
	"DescribeInstanceInformation", "DescribeInstanceStatus", "DescribeInstanceTypes", "DescribeInstances",
	"DescribeInternetGateways", "DescribeNatGateways", "DescribeNodegroup", "DescribeRepositories",
	"DescribeRouteTables", "DescribeScalableTargets", "DescribeScalingPolicies", "DescribeSecret",
	"DescribeServices", "DescribeSnapshots", "DescribeSubnets", "DescribeTargetHealth", "DescribeTaskDefinition",
	"DescribeTasks", "DescribeTransitGatewayAttachments", "DescribeTransitGateways", "DescribeVolumes",
	"DescribeVpcAttribute", "DescribeVpcEndpoints", "DescribeVpcs",
	"GetBucketEncryption", "GetBucketLifecycleConfiguration", "GetBucketLocation", "GetBucketLogging",
	"GetBucketNotificationConfiguration", "GetBucketPolicy", "GetBucketReplication", "GetBucketTagging",
	"GetBucketVersioning", "GetCallerIdentity", "GetConsoleOutput", "GetConsoleScreenshot", "GetDeploymentGroup",
	"GetLifecyclePolicy", "GetLifecyclePolicyPreview", "GetLogEvents", "GetMetricData", "GetMetricStatistics",
	"GetObject", "GetObjectLockConfiguration", "GetObjectTagging", "GetPublicAccessBlock", "GetRepositoryPolicy",
	"GetResourcePolicy", "GetSecretValue", "HeadObject",
	"ListAccessEntries", "ListAddons", "ListApplications", "ListBuckets", "ListClusters", "ListContainerInstances",
	"ListDeploymentGroups", "ListFargateProfiles", "ListFoundationModels", "ListInferenceProfiles", "ListInsights",
	"ListMultipartUploads", "ListNodegroups", "ListObjectVersions", "ListObjectsV2", "ListPodIdentityAssociations",
	"ListSecretVersionIds", "ListSecrets", "ListServices", "ListTaskDefinitionFamilies", "ListTaskDefinitions",
	"ListTasks",

	// Inference: the chat asks a model a question. Off by default, and its command-running backend is refused elsewhere because it never touches this stack.
	"ConverseStream",
}

var mutatingOperations = []string{
	"AbortMultipartUpload", "AssociateAddress", "BatchDeleteImage", "CopyObject", "CreateBucket", "CreateImage",
	"CreateSnapshot", "DeleteBucket", "DeleteCluster", "DeleteObject", "DeleteRepository", "DeleteSecret",
	"DeleteService", "DisassociateAddress", "ModifyInstanceAttribute", "PutBucketVersioning",
	"PutImageScanningConfiguration", "PutImageTagMutability", "PutLifecyclePolicy", "RebootInstances",
	"RemoveRegionsFromReplication", "ReplicateSecretToRegions", "RestoreSecret", "RotateSecret",
	"SendSSHPublicKey", "StartInstances", "StartLifecyclePolicyPreview", "StopInstances", "StopTask",
	"TerminateInstances", "UpdateClusterVersion", "UpdateService",
}

// sdkCall matches a call on one of Client's service clients, and sdkPaginator the paginators that make the same calls a page at a time.
var (
	sdkCall      = regexp.MustCompile(`\bc\.(?:EC2|EC2InstanceConnect|S3|EKS|ECS|ECR|Secrets|SSM|CloudWatch|CloudWatchLogs|ELBv2|CodeDeploy|ApplicationAutoScaling|AutoScaling|STS|BedrockRuntime|Bedrock)\.([A-Z][A-Za-z0-9]*)\(`)
	sdkPaginator = regexp.MustCompile(`\bNew([A-Z][A-Za-z0-9]*)Paginator\(`)
)

// This is the test that keeps the promise true tomorrow. It reads every AWS operation the package calls out of the source and requires each one to be classified above, then checks the policy agrees with that classification.
// A new mutating call is refused by the default anyway; what this catches is the two ways that goes wrong later: a read the policy would refuse, which breaks a panel for everyone running without the flag, and a mutation somebody assumed was a read.
func TestEveryOperationTheCodeCallsIsClassified(t *testing.T) {
	called := operationsCalledInPackage(t)
	if len(called) == 0 {
		t.Fatal("found no AWS calls in the package, so this test is asserting nothing; the source scan must have stopped matching")
	}

	classified := map[string]bool{}
	for _, operation := range readOnlyOperations {
		classified[operation] = true
	}
	for _, operation := range mutatingOperations {
		if _, duplicate := classified[operation]; duplicate {
			t.Errorf("%s is listed as both a read and a mutation", operation)
		}
		classified[operation] = false
	}

	for _, operation := range called {
		read, listed := classified[operation]
		if !listed {
			t.Errorf("%s is called but not classified: add it to readOnlyOperations or mutatingOperations in this file, having checked which it is", operation)
			continue
		}
		if got := readOperation(operation); got != read {
			t.Errorf("%s is classified read=%v but the policy says read=%v", operation, read, got)
		}
	}

	// The lists are the record of a decision, so an entry left behind after the call it described was deleted is a decision about nothing.
	inSource := map[string]bool{}
	for _, operation := range called {
		inSource[operation] = true
	}
	for operation := range classified {
		if !inSource[operation] {
			t.Errorf("%s is classified but no longer called anywhere; drop it from the list", operation)
		}
	}
}

func operationsCalledInPackage(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}

	found := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		for _, match := range sdkCall.FindAllStringSubmatch(string(source), -1) {
			found[match[1]] = true
		}
		for _, match := range sdkPaginator.FindAllStringSubmatch(string(source), -1) {
			found[match[1]] = true
		}
	}

	operations := make([]string, 0, len(found))
	for operation := range found {
		operations = append(operations, operation)
	}
	sort.Strings(operations)

	return operations
}

// handlerFunc counts a request and answers with nothing, which is all the guard tests need: they assert on arrival, not on the reply.
type handlerFunc func()

func (h handlerFunc) ServeHTTP(_ http.ResponseWriter, _ *http.Request) { h() }

// The SDK guard cannot see a child process, so anything in this package that runs one carries requireWrites instead.
// Scanned rather than listed, because the failure this prevents is somebody adding a fourth such function and nobody noticing it answers to nothing.
func TestEveryFunctionThatRunsAChildProcessAsksForWrites(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}

	executes := regexp.MustCompile(`cmd\.(Run|Start|Output|CombinedOutput)\(\)`)
	checked := 0

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}

		// Split on the function keyword at column zero: gofmt guarantees that shape, so each piece is one function body.
		for _, block := range strings.Split(string(source), "\nfunc ") {
			if !executes.MatchString(block) {
				continue
			}
			checked++
			if !strings.Contains(block, "requireWrites(") {
				signature := strings.SplitN(block, "{", 2)[0]
				t.Errorf("%s: func %s runs a child process without requireWrites, so it would run in a read-only session", name, strings.TrimSpace(signature))
			}
		}
	}

	if checked == 0 {
		t.Error("found no functions running child processes, so the scan has stopped matching and is asserting nothing")
	}
}

func TestTheThingsThatRunOutsideTheSDKRefuseWithoutTheFlag(t *testing.T) {
	client := &Client{Region: "eu-west-1"}

	for name, call := range map[string]func() error{
		"LaunchSSMSession": func() error { return client.LaunchSSMSession("i-0123456789abcdef0", "eu-west-1") },
		"StartPortForward": func() error { return client.StartPortForward("i-0123456789abcdef0", "eu-west-1", 8080, 80, "") },
		"UpdateKubeconfig": func() error { return client.UpdateKubeconfig(context.Background(), "cluster") },
	} {
		if err := call(); !errors.Is(err, ErrReadOnly) {
			t.Errorf("%s error = %v, want it to unwrap to ErrReadOnly", name, err)
		}
	}
}
