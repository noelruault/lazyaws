package resources

import "testing"

// Resolve and Matches run on every keystroke in the command bar, so their cost is felt as input latency rather than as throughput.

func BenchmarkResolveExact(b *testing.B) {
	reg := testRegistry()
	b.ReportAllocs()

	for b.Loop() {
		if _, err := reg.Resolve(":ecs"); err != nil {
			b.Fatal(err)
		}
	}
}

// A miss on the exact and prefix paths falls through to fuzzy ranking, the expensive branch.
func BenchmarkResolveFuzzyFallback(b *testing.B) {
	reg := testRegistry()
	b.ReportAllocs()

	for b.Loop() {
		if _, err := reg.Resolve(":scrts"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMatchesPrefix(b *testing.B) {
	reg := testRegistry()
	b.ReportAllocs()

	for b.Loop() {
		_ = reg.Matches(":e")
	}
}

// The empty needle is the worst case: every registered name is a candidate.
func BenchmarkMatchesEmpty(b *testing.B) {
	reg := testRegistry()
	b.ReportAllocs()

	for b.Loop() {
		_ = reg.Matches("")
	}
}
