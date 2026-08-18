package resources

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func testRegistry() *Registry {
	reg := NewRegistry("aws")
	noop := func(Ref) error { return nil }

	reg.Register(
		&Entry{Ref: Ref{Provider: "aws", Service: "profiles"}, Title: "Profiles", Aliases: []string{"profile"}, Focus: noop},
		&Entry{Ref: Ref{Provider: "aws", Service: "ecs", Resource: "clusters"}, Title: "ECS Clusters", Aliases: []string{"ecs"}, Focus: noop},
		&Entry{Ref: Ref{Provider: "aws", Service: "ecs", Resource: "services"}, Title: "ECS Services", Focus: noop},
		&Entry{Ref: Ref{Provider: "aws", Service: "ec2", Resource: "instances"}, Title: "EC2 Instances", Aliases: []string{"ec2"}, Focus: noop},
		&Entry{Ref: Ref{Provider: "aws", Service: "ecr", Resource: "repositories"}, Title: "ECR Repositories", Aliases: []string{"ecr"}, Focus: noop},
		&Entry{Ref: Ref{Provider: "aws", Service: "eks", Resource: "clusters"}, Title: "EKS Clusters", Aliases: []string{"eks"}, Focus: noop},
		&Entry{Ref: Ref{Provider: "aws", Service: "s3", Resource: "buckets"}, Title: "S3 Buckets", Aliases: []string{"s3"}, Focus: noop},
		&Entry{Ref: Ref{Provider: "aws", Service: "cloudwatch", Resource: "log-groups"}, Title: "Log Groups", Aliases: []string{"logs"}, Focus: noop},
		&Entry{Ref: Ref{Provider: "aws", Service: "billing", Resource: "invoices"}, Title: "Invoices"},
	)

	return reg
}

func TestResolve(t *testing.T) {
	reg := testRegistry()

	for _, tc := range []struct {
		name  string
		input string
		want  Ref
	}{
		{"alias", ":ecs", Ref{Provider: "aws", Service: "ecs", Resource: "clusters"}},
		{"alias without the colon", "ecs", Ref{Provider: "aws", Service: "ecs", Resource: "clusters"}},
		{"full ref", ":aws:ecs:services", Ref{Provider: "aws", Service: "ecs", Resource: "services"}},
		{"service and resource, provider implied", ":ecs:services", Ref{Provider: "aws", Service: "ecs", Resource: "services"}},
		{"provider and service, default resource", ":aws:ecs", Ref{Provider: "aws", Service: "ecs", Resource: "clusters"}},
		{"empty segments collapse", "::ecs", Ref{Provider: "aws", Service: "ecs", Resource: "clusters"}},
		{"uppercase", ":ECS", Ref{Provider: "aws", Service: "ecs", Resource: "clusters"}},
		{"service with no resource", ":profiles", Ref{Provider: "aws", Service: "profiles"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := reg.Resolve(tc.input)
			if err != nil {
				t.Fatalf("Resolve(%q): %v", tc.input, err)
			}
			if got.Key() != tc.want.Key() {
				t.Fatalf("Resolve(%q) = %v, want %v", tc.input, got.Key(), tc.want.Key())
			}
			if len(got.Path) != 0 {
				t.Fatalf("Resolve(%q) picked up a selector path %v", tc.input, got.Path)
			}
		})
	}
}

func TestResolveCapturesSelectorPath(t *testing.T) {
	reg := testRegistry()

	for _, tc := range []struct {
		input    string
		wantKey  Key
		wantPath []string
	}{
		{":aws:profiles:staging", Key{"aws", "profiles", ""}, []string{"staging"}},
		{":ecs:web-cluster:web", Key{"aws", "ecs", "clusters"}, []string{"web-cluster", "web"}},
		// Resource selectors must preserve embedded slashes.
		{":aws:cloudwatch:log-groups:/aws/lambda/my-fn", Key{"aws", "cloudwatch", "log-groups"}, []string{"/aws/lambda/my-fn"}},
	} {
		got, err := reg.Resolve(tc.input)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", tc.input, err)
		}
		if got.Key() != tc.wantKey {
			t.Errorf("Resolve(%q) key = %v, want %v", tc.input, got.Key(), tc.wantKey)
		}
		if !slices.Equal(got.Path, tc.wantPath) {
			t.Errorf("Resolve(%q) path = %v, want %v", tc.input, got.Path, tc.wantPath)
		}
	}
}

func TestResolveUniquePrefix(t *testing.T) {
	reg := testRegistry()

	got, err := reg.Resolve(":cloudw")
	if err != nil {
		t.Fatalf("Resolve(:cloudw): %v", err)
	}
	if want := (Key{"aws", "cloudwatch", "log-groups"}); got.Key() != want {
		t.Fatalf("Resolve(:cloudw) = %v, want %v", got.Key(), want)
	}
}

// Ambiguous prefixes must fail instead of navigating arbitrarily.
func TestResolveRefusesToGuess(t *testing.T) {
	reg := testRegistry()

	if _, err := reg.Resolve(":e"); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("Resolve(:e) error = %v, want ErrAmbiguous", err)
	}
	if _, err := reg.Resolve(":ec"); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("Resolve(:ec) error = %v, want ErrAmbiguous", err)
	}
}

func TestResolveEmptyAndUnknown(t *testing.T) {
	reg := testRegistry()

	if _, err := reg.Resolve(":"); !errors.Is(err, ErrEmpty) {
		t.Fatalf("Resolve(:) error = %v, want ErrEmpty", err)
	}
	if _, err := reg.Resolve(""); !errors.Is(err, ErrEmpty) {
		t.Fatalf("Resolve(\"\") error = %v, want ErrEmpty", err)
	}
	if _, err := reg.Resolve(":zzzzz"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("Resolve(:zzzzz) error = %v, want ErrUnknown", err)
	}
}

// Fuzzy matching must remain behind exact and prefix resolution.
func TestResolveFallsBackToFuzzy(t *testing.T) {
	reg := testRegistry()

	got, err := reg.Resolve(":esvc")
	if err != nil {
		t.Fatalf("Resolve(:esvc): %v", err)
	}
	if want := (Key{"aws", "ecs", "services"}); got.Key() != want {
		t.Fatalf("Resolve(:esvc) = %v, want %v", got.Key(), want)
	}
}

// Fuzzy scoring must never outrank an exact alias.
func TestExactBeatsFuzzy(t *testing.T) {
	reg := NewRegistry("aws")
	noop := func(Ref) error { return nil }
	reg.Register(
		&Entry{Ref: Ref{Provider: "aws", Service: "s3", Resource: "buckets"}, Title: "S3", Aliases: []string{"s3"}, Focus: noop},
		&Entry{Ref: Ref{Provider: "aws", Service: "secretsmanager", Resource: "secrets"}, Title: "Secrets", Aliases: []string{"s3cr3ts"}, Focus: noop},
	)

	got, err := reg.Resolve(":s3")
	if err != nil {
		t.Fatalf("Resolve(:s3): %v", err)
	}
	if got.Service != "s3" {
		t.Fatalf("Resolve(:s3) = %v, want the s3 alias to win outright", got.Key())
	}
}

func TestSuggestions(t *testing.T) {
	reg := testRegistry()

	got := reg.Suggestions(":ec")
	if len(got) == 0 {
		t.Fatal("Suggestions(:ec) returned nothing")
	}
	for _, name := range got {
		if !strings.HasPrefix(name, "ec") {
			t.Errorf("Suggestions(:ec) offered %q", name)
		}
	}

	// Equal results must stay stable between keystrokes.
	if second := reg.Suggestions(":ec"); !slices.Equal(got, second) {
		t.Errorf("Suggestions is not stable: %v then %v", got, second)
	}

	if n := len(reg.Suggestions("")); n > MaxSuggestions {
		t.Errorf("Suggestions(\"\") returned %d names, capped at %d", n, MaxSuggestions)
	}
}

func TestCommonPrefixExpandsToTheSharedStem(t *testing.T) {
	reg := NewRegistry("aws")
	noop := func(Ref) error { return nil }
	reg.Register(
		&Entry{Ref: Ref{Provider: "aws", Service: "ecs", Resource: "clusters"}, Title: "ECS", Aliases: []string{"ecs"}, Focus: noop},
		&Entry{Ref: Ref{Provider: "aws", Service: "ec2", Resource: "instances"}, Title: "EC2", Aliases: []string{"ec2"}, Focus: noop},
	)

	if got := CommonPrefix(reg.Matches(":e")); got != "ec" {
		t.Errorf("CommonPrefix(:e) = %q, want %q", got, "ec")
	}
	if got := CommonPrefix(reg.Matches(":ecs")); !strings.HasPrefix(got, "ecs") {
		t.Errorf("CommonPrefix(:ecs) = %q, want something starting with ecs", got)
	}
}

// Completion must consider candidates hidden by the display cap.
func TestCompletionSeesPastTheSuggestionCap(t *testing.T) {
	reg := NewRegistry("aws")
	noop := func(Ref) error { return nil }

	for _, service := range []string{"athena", "acm", "appsync", "apigateway", "apprunner", "autoscaling", "accessanalyzer", "appmesh", "amplify", "appconfig"} {
		reg.Register(&Entry{Ref: Ref{Provider: "aws", Service: service, Resource: "things"}, Title: service, Focus: noop})
	}
	reg.Register(&Entry{Ref: Ref{Provider: "aws", Service: "ask", Resource: "questions"}, Title: "Ask", Aliases: []string{"ask"}, Focus: noop})

	if n := len(reg.Suggestions(":a")); n > MaxSuggestions {
		t.Fatalf("Suggestions returned %d, want at most %d", n, MaxSuggestions)
	}

	completion := CommonPrefix(reg.Matches(":a"))
	for _, name := range reg.Matches(":a") {
		if !strings.HasPrefix(name, completion) {
			t.Fatalf("tab would complete %q to %q, which %q no longer starts with", ":a", completion, name)
		}
	}
}

func TestFocusRef(t *testing.T) {
	reg := testRegistry()

	var focused Ref
	entry, _ := reg.Get(Key{"aws", "ecs", "clusters"})
	entry.Focus = func(ref Ref) error {
		focused = ref
		return nil
	}

	ref, err := reg.Resolve(":ecs:my-cluster")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := reg.FocusRef(ref); err != nil {
		t.Fatalf("FocusRef: %v", err)
	}
	if !slices.Equal(focused.Path, []string{"my-cluster"}) {
		t.Fatalf("Focus got path %v, want the selector to be handed through", focused.Path)
	}
}

func TestFocusRefOnAnActionOnlyResource(t *testing.T) {
	reg := testRegistry()

	ref, err := reg.Resolve(":billing")
	if err != nil {
		t.Fatalf("Resolve(:billing): %v", err)
	}
	if err := reg.FocusRef(ref); !errors.Is(err, ErrNotNavigable) {
		t.Fatalf("FocusRef error = %v, want ErrNotNavigable", err)
	}
}

// Duplicate aliases must fail at registration instead of shadowing a destination.
func TestDuplicatesPanicAtRegistration(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []*Entry
	}{
		{
			"same ref twice",
			[]*Entry{
				{Ref: Ref{Provider: "aws", Service: "ecs", Resource: "clusters"}, Title: "one"},
				{Ref: Ref{Provider: "aws", Service: "ecs", Resource: "clusters"}, Title: "two"},
			},
		},
		{
			"same alias twice",
			[]*Entry{
				{Ref: Ref{Provider: "aws", Service: "ecs", Resource: "clusters"}, Title: "one", Aliases: []string{"c"}},
				{Ref: Ref{Provider: "aws", Service: "ec2", Resource: "instances"}, Title: "two", Aliases: []string{"c"}},
			},
		},
		{
			"entry carrying a selector path",
			[]*Entry{{Ref: Ref{Provider: "aws", Service: "ecs", Resource: "clusters", Path: []string{"x"}}, Title: "one"}},
		},
		{
			"entry with no provider",
			[]*Entry{{Ref: Ref{Service: "ecs"}, Title: "one"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected a panic at registration")
				}
			}()
			NewRegistry("aws").Register(tc.entries...)
		})
	}
}

func TestEntriesFollowRegistrationOrder(t *testing.T) {
	want := []string{
		"Profiles", "ECS Clusters", "ECS Services", "EC2 Instances",
		"ECR Repositories", "EKS Clusters", "S3 Buckets", "Log Groups", "Invoices",
	}

	got := make([]string, 0, len(want))
	for _, entry := range testRegistry().Entries() {
		got = append(got, entry.Title)
	}

	if !slices.Equal(got, want) {
		t.Errorf("Entries() = %v, want registration order %v", got, want)
	}
}

// Registration order is what puts the common panels at the top of the command bar; alphabetical would bury them.
func TestMatchOrderFollowsRegistrationOrder(t *testing.T) {
	noop := func(Ref) error { return nil }
	reg := NewRegistry("aws")
	reg.Register(
		&Entry{Ref: Ref{Provider: "aws", Service: "zebra"}, Title: "Zebra", Aliases: []string{"zz-second"}, Focus: noop},
		&Entry{Ref: Ref{Provider: "aws", Service: "alpha"}, Title: "Alpha", Aliases: []string{"zz-first"}, Focus: noop},
	)

	want := []string{"zz-second", "zz-first"}
	if got := reg.Matches(":zz-"); !slices.Equal(got, want) {
		t.Errorf("Matches(:zz-) = %v, want registration order %v", got, want)
	}
}

// Two providers exposing the same service must coexist: the bare shortcut goes to whoever registered first.
func TestDerivedShortcutsYieldToFirstProvider(t *testing.T) {
	noop := func(Ref) error { return nil }
	reg := NewRegistry("aws")
	reg.Register(
		&Entry{Ref: Ref{Provider: "aws", Service: "buckets", Resource: "all"}, Title: "AWS Buckets", Focus: noop},
		&Entry{Ref: Ref{Provider: "gcp", Service: "buckets", Resource: "all"}, Title: "GCP Buckets", Focus: noop},
	)

	first, err := reg.Resolve(":buckets")
	if err != nil {
		t.Fatalf("Resolve(:buckets) = %v", err)
	}
	if first.Provider != "aws" {
		t.Errorf("bare shortcut resolved to %q, want the first registrant %q", first.Provider, "aws")
	}

	// The provider that lost the shortcut must stay reachable by its qualified name.
	second, err := reg.Resolve(":gcp:buckets")
	if err != nil {
		t.Fatalf("Resolve(:gcp:buckets) = %v", err)
	}
	if second.Provider != "gcp" {
		t.Errorf("qualified ref resolved to %q, want %q", second.Provider, "gcp")
	}
}

// An explicit alias claimed by two providers is a wiring bug, not a precedence question.
func TestExplicitAliasCollisionAcrossProvidersPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic when two providers claim the same explicit alias")
		}
	}()

	noop := func(Ref) error { return nil }
	NewRegistry("aws").Register(
		&Entry{Ref: Ref{Provider: "aws", Service: "storage"}, Title: "AWS", Aliases: []string{"store"}, Focus: noop},
		&Entry{Ref: Ref{Provider: "gcp", Service: "storage"}, Title: "GCP", Aliases: []string{"store"}, Focus: noop},
	)
}

func TestRefString(t *testing.T) {
	for _, tc := range []struct {
		ref  Ref
		want string
	}{
		{Ref{Provider: "aws", Service: "ecs", Resource: "clusters"}, ":aws:ecs:clusters"},
		{Ref{Provider: "aws", Service: "profiles"}, ":aws:profiles"},
		{Ref{Provider: "aws", Service: "profiles", Path: []string{"staging"}}, ":aws:profiles:staging"},
	} {
		if got := tc.ref.String(); got != tc.want {
			t.Errorf("Ref.String() = %q, want %q", got, tc.want)
		}
	}
}

func TestActionValid(t *testing.T) {
	run := func(context.Context, string) error { return nil }

	for _, tc := range []struct {
		name    string
		action  Action
		wantErr bool
	}{
		{"ok", Action{Name: "Stop", Run: run}, false},
		{"dangerous with a token", Action{Name: "Terminate", Confirm: ConfirmDangerous, Token: "web-1 (i-0abc)", Run: run}, false},
		{"dangerous without a token", Action{Name: "Terminate", Confirm: ConfirmDangerous, Run: run}, true},
		{"no run func", Action{Name: "Stop"}, true},
		{"no name", Action{Run: run}, true},
	} {
		if err := tc.action.Valid(); (err != nil) != tc.wantErr {
			t.Errorf("%s: Valid() = %v, wantErr %v", tc.name, err, tc.wantErr)
		}
	}
}
