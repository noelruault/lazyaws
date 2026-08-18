package fuzzy

import (
	"slices"
	"testing"
)

var refs = []string{
	"aws:ecs:clusters",
	"aws:ecs:services",
	"aws:ecs:tasks",
	"aws:ecs:task-definitions",
	"aws:ec2:instances",
	"aws:ec2:security-groups",
	"aws:ec2:volumes",
	"aws:ecr:repositories",
	"aws:eks:clusters",
	"aws:eks:node-groups",
	"aws:s3:buckets",
	"aws:vpc:subnets",
	"aws:vpc:vpcs",
	"aws:cloudwatch:log-groups",
	"aws:secretsmanager:secrets",
	"aws:profiles",
}

func TestMatchRejectsNonSubsequence(t *testing.T) {
	if _, _, ok := Match("zzz", "aws:ecs:clusters"); ok {
		t.Fatal("zzz should not match aws:ecs:clusters")
	}
	if _, _, ok := Match("sce", "aws:ecs"); ok {
		t.Fatal("sce should not match aws:ecs")
	}
}

func TestEmptyPatternMatchesEverything(t *testing.T) {
	score, positions, ok := Match("", "aws:ecs:clusters")
	if !ok || score != 0 || positions != nil {
		t.Fatalf("empty pattern: got (%d, %v, %v), want (0, nil, true)", score, positions, ok)
	}
}

func TestPositionsPointAtTheMatchedCharacters(t *testing.T) {
	_, positions, ok := Match("subn", "aws:vpc:subnets")
	if !ok {
		t.Fatal("subn should match aws:vpc:subnets")
	}
	want := []int{8, 9, 10, 11}
	if !slices.Equal(positions, want) {
		t.Fatalf("positions = %v, want %v", positions, want)
	}
	for i, p := range positions {
		if got := []rune("aws:vpc:subnets")[p]; got != []rune("subn")[i] {
			t.Fatalf("position %d points at %q, want %q", p, got, []rune("subn")[i])
		}
	}
}

func TestRankFindsTheIntendedResource(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		want    string
	}{
		{"esvc", "aws:ecs:services"},
		{"subn", "aws:vpc:subnets"},
		{"logs", "aws:cloudwatch:log-groups"},
		{"ecscl", "aws:ecs:clusters"},
		{"s3b", "aws:s3:buckets"},
		{"secr", "aws:secretsmanager:secrets"},
		{"ng", "aws:eks:node-groups"},
		{"prof", "aws:profiles"},
	} {
		results := Rank(tc.pattern, refs)
		if len(results) == 0 {
			t.Errorf("%q matched nothing", tc.pattern)
			continue
		}
		if results[0].Text != tc.want {
			t.Errorf("%q ranked %q first, want %q (full order: %v)", tc.pattern, results[0].Text, tc.want, texts(results))
		}
	}
}

// Boundary hits must outrank incidental matches.
func TestBoundaryMatchesOutrankIncidentalOnes(t *testing.T) {
	atBoundary, _, ok := Match("sec", "aws:ec2:security-groups")
	if !ok {
		t.Fatal("sec should match aws:ec2:security-groups")
	}
	scattered, _, ok := Match("sec", "aws:ecs:services")
	if !ok {
		t.Fatal("sec should match aws:ecs:services")
	}
	if atBoundary <= scattered {
		t.Fatalf("boundary match scored %d, scattered match scored %d: the boundary bonus is not being applied", atBoundary, scattered)
	}
}

// Consecutive hits must outrank the same characters scattered through equal-length text.
func TestConsecutiveRunBeatsScatteredHits(t *testing.T) {
	run, _, _ := Match("abc", "xxabcxx")
	spread, _, _ := Match("abc", "xaxbxcx")
	if run <= spread {
		t.Fatalf("consecutive run scored %d, spread scored %d", run, spread)
	}
}

func TestRankIsCaseInsensitive(t *testing.T) {
	results := Rank("ECS", refs)
	if len(results) == 0 {
		t.Fatal("uppercase pattern matched nothing")
	}
	lower := Rank("ecs", refs)
	if !slices.Equal(texts(results), texts(lower)) {
		t.Fatalf("case changed the ranking: %v vs %v", texts(results), texts(lower))
	}
}

// Equal scores must remain stable while the list re-renders between keystrokes.
func TestRankOrderIsStable(t *testing.T) {
	first := texts(Rank("e", refs))
	for range 5 {
		if got := texts(Rank("e", refs)); !slices.Equal(got, first) {
			t.Fatalf("ranking changed between identical calls:\n%v\n%v", first, got)
		}
	}
}

func TestRankDropsNonMatches(t *testing.T) {
	for _, r := range Rank("esvc", refs) {
		if _, _, ok := Match("esvc", r.Text); !ok {
			t.Fatalf("Rank returned %q which does not match", r.Text)
		}
	}
	if got := len(Rank("zzzz", refs)); got != 0 {
		t.Fatalf("Rank returned %d results for a pattern that matches nothing", got)
	}
}

func texts(results []Result) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.Text
	}
	return out
}
