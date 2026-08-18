package ui

import (
	"strings"
	"testing"

	"github.com/fatih/color"
)

// The chat re-renders the whole accumulated transcript on every streamed line, so these costs are paid per token received, and they grow with conversation length.

const benchAnswer = "Here is what the cluster is doing right now, in some detail.\n\n" +
	"```go\nfunc main() {\n\tfmt.Println(\"a reasonably long line of code that will need wrapping at narrow widths\")\n}\n```\n\n" +
	"The **service** is running `3` tasks and the _deployment_ finished a moment ago.\n"

func benchTranscript(turns int) string {
	var b strings.Builder
	for range turns {
		b.WriteString(benchAnswer)
	}

	return b.String()
}

// fatih/color disables itself when stdout is not a terminal, which it never is under `go test`.
// Without this the render benchmarks skip every escape sequence and measure a path no production run takes, because gocui always owns a tty.
func benchForceColor(b *testing.B) {
	b.Helper()

	previous := color.NoColor
	b.Cleanup(func() { color.NoColor = previous })
	color.NoColor = false
}

func BenchmarkRenderQMarkdownShort(b *testing.B) {
	benchForceColor(b)
	text := benchTranscript(1)
	// The map is test infrastructure: allocating it per iteration would inflate the reported count over what the render itself costs in production.
	open := map[int]bool{}
	b.ReportAllocs()

	for b.Loop() {
		_ = renderQMarkdown(text, 100, open)
	}
}

// A long conversation is the case that decides whether streaming stays smooth.
func BenchmarkRenderQMarkdownLong(b *testing.B) {
	benchForceColor(b)
	text := benchTranscript(50)
	open := map[int]bool{}
	b.ReportAllocs()

	for b.Loop() {
		_ = renderQMarkdown(text, 100, open)
	}
}

// A narrow panel is where wrapping multiplies rows past the preallocated capacity.
func BenchmarkRenderQMarkdownLongNarrow(b *testing.B) {
	benchForceColor(b)
	text := benchTranscript(50)
	open := map[int]bool{}
	b.ReportAllocs()

	for b.Loop() {
		_ = renderQMarkdown(text, 40, open)
	}
}

// Folded blocks skip their bodies; this is the payoff the fold model is meant to buy.
func BenchmarkRenderQMarkdownLongFolded(b *testing.B) {
	benchForceColor(b)
	text := benchTranscript(50)
	folded := map[int]bool{}
	for i := range 50 {
		folded[i] = true
	}
	b.ReportAllocs()

	for b.Loop() {
		_ = renderQMarkdown(text, 100, folded)
	}
}

// The panel joins the rows before handing them to gocui, so the join is part of the per-streamed-line cost even though it sits outside renderQMarkdown.
func BenchmarkRenderQMarkdownLongPlusString(b *testing.B) {
	benchForceColor(b)
	text := benchTranscript(50)
	open := map[int]bool{}
	b.ReportAllocs()

	for b.Loop() {
		_ = renderQMarkdown(text, 100, open).String()
	}
}

func BenchmarkWrapLineNoWrap(b *testing.B) {
	line := "a line that already fits inside the panel width"
	b.ReportAllocs()

	for b.Loop() {
		_ = wrapLine(line, 100)
	}
}

func BenchmarkWrapLineManyRows(b *testing.B) {
	line := strings.Repeat("wrap me across many rows ", 40)
	b.ReportAllocs()

	for b.Loop() {
		_ = wrapLine(line, 40)
	}
}

// Double-width runes force the width accounting down its slow path.
func BenchmarkWrapLineWideRunes(b *testing.B) {
	line := strings.Repeat("日本語のテキストを折り返す", 20)
	b.ReportAllocs()

	for b.Loop() {
		_ = wrapLine(line, 40)
	}
}

func BenchmarkStyleInlineNoMarkers(b *testing.B) {
	row := "a plain sentence with no inline markup at all in it"
	b.ReportAllocs()

	for b.Loop() {
		_ = styleInline(row)
	}
}

func BenchmarkStyleInlineAllMarkers(b *testing.B) {
	benchForceColor(b)
	row := "the **service** is running `3` tasks and the *deployment* finished"
	b.ReportAllocs()

	for b.Loop() {
		_ = styleInline(row)
	}
}
