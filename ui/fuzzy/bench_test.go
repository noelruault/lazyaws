package fuzzy

import "testing"

// Rank is the command bar's fallback path: it scores every registered name against the needle on each keystroke, so its cost scales with the registry, not with one match.
var benchCandidates = []string{
	"aws:profiles", "aws:ecs:clusters", "aws:ecs:services", "aws:ecs:tasks",
	"aws:ec2:instances", "aws:s3:buckets", "aws:eks:clusters", "aws:ecr:repositories",
	"aws:secretsmanager:secrets", "aws:cloudwatch:log-groups", "aws:amazon-q",
	"aws:settings", "profiles", "ecs", "ec2", "s3", "eks", "ecr", "secrets", "logs",
}

func BenchmarkMatchHit(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, _, ok := Match("scrts", "aws:secretsmanager:secrets"); !ok {
			b.Fatal("expected a match")
		}
	}
}

// A miss still walks the whole text before giving up.
func BenchmarkMatchMiss(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, _, ok := Match("zzzz", "aws:secretsmanager:secrets"); ok {
			b.Fatal("expected no match")
		}
	}
}

func BenchmarkRankRegistrySized(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = Rank("ec", benchCandidates)
	}
}
