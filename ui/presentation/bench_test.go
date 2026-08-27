package presentation

import (
	"strconv"
	"strings"
	"testing"

	"github.com/fatih/color"
)

// fatih/color disables itself when stdout is not a terminal, which it never is under `go test`.
// Without this the render benchmarks skip every escape sequence and measure a path no production run takes, because gocui always owns a tty.
func benchForceColor(b *testing.B) {
	b.Helper()

	previous := color.NoColor
	b.Cleanup(func() { color.NoColor = previous })
	color.NoColor = false
}

// benchBlock is a section-shaped block: a heading and rows, which is what the overviews hand the zipper.
func benchBlock(prefix string, lines int) string {
	rows := make([]string, lines)
	for i := range rows {
		rows[i] = prefix + " row " + strconv.Itoa(i) + ": some value worth reading"
	}

	return strings.Join(rows, "\n")
}

// Columns runs once per overview render, and the two-column path is the one that interleaves and cuts line by line.
func BenchmarkColumns(b *testing.B) {
	left, right := benchBlock("left", 40), benchBlock("right", 40)
	b.ReportAllocs()

	for b.Loop() {
		_ = Columns(overviewWidth, 1, left, right)
	}
}

// The stacked path below minTwoColWidth is different code, not a cheaper version of the same code.
func BenchmarkColumnsStacked(b *testing.B) {
	left, right := benchBlock("left", 40), benchBlock("right", 40)
	b.ReportAllocs()

	for b.Loop() {
		_ = Columns(stackedWidth, 1, left, right)
	}
}

// The fixtures are built once: allocating them per iteration would report the fixture's cost as the formatter's.
// They are the same full fixtures the render tests use, so a formatter measured here is measured with every section answered, which is the expensive case and the one on screen.
func BenchmarkFormatInstanceOverview(b *testing.B) {
	benchForceColor(b)
	instance, overview := overviewInstance(), fullOverview()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatInstanceOverview(instance, overview, overviewWidth, overviewNow)
	}
}

// A narrow terminal lays every section out whole instead of cutting it to a column, so it renders more text, not less.
func BenchmarkFormatInstanceOverviewStacked(b *testing.B) {
	benchForceColor(b)
	instance, overview := overviewInstance(), fullOverview()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatInstanceOverview(instance, overview, stackedWidth, overviewNow)
	}
}

func BenchmarkFormatECSClusterOverview(b *testing.B) {
	benchForceColor(b)
	cluster, overview := clusterFixture()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatECSClusterOverview(cluster, overview, overviewWidth)
	}
}

func BenchmarkFormatECSServiceOverview(b *testing.B) {
	benchForceColor(b)
	service, overview, now := serviceFixture()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatECSServiceOverview(service, overview, overviewWidth, now)
	}
}

func BenchmarkFormatBucketOverview(b *testing.B) {
	benchForceColor(b)
	bucket, overview := overviewBucket(), fullBucketOverview()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatBucketOverview(bucket, overview, overviewWidth, overviewNow)
	}
}

func BenchmarkFormatECRRepositoryOverview(b *testing.B) {
	benchForceColor(b)
	repository, images := overviewRepository(), overviewImages()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatECRRepositoryOverview(repository, images, nil, overviewWidth, overviewNow)
	}
}

func BenchmarkFormatVPCOverview(b *testing.B) {
	benchForceColor(b)
	vpc, overview := overviewVPC(), fullVPCOverview()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatVPCOverview(vpc, overview, overviewWidth)
	}
}

func BenchmarkFormatEKSClusterOverview(b *testing.B) {
	benchForceColor(b)
	cluster, overview := overviewEKSCluster(), fullEKSOverview()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatEKSClusterOverview(cluster, overview, overviewWidth)
	}
}

func BenchmarkFormatSecretOverview(b *testing.B) {
	benchForceColor(b)
	secret := rotatingSecret()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatSecretOverview(secret, overviewWidth, overviewNow)
	}
}
