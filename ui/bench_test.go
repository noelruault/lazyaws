package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/presentation"
	"github.com/noelruault/lazyaws/ui/utils"
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

// The list rerender is what a refresh tick costs when nothing changed, and it runs inside the gocui update closure, so it is paid on the UI thread.
// It mirrors SideListPanel.renderTable rather than calling it: driving the panel needs a view with a width, and what is being measured is the cells-plus-fit composition, not gocui.
func BenchmarkRerenderListEC2(b *testing.B) {
	benchForceColor(b)

	instances := make([]*aws.Instance, 100)
	for i := range instances {
		instances[i] = &aws.Instance{
			ID:           "i-0abcdef12345678" + strconv.Itoa(i),
			Name:         "service-with-a-fairly-long-name-" + strconv.Itoa(i),
			State:        "running",
			InstanceType: "t3a.micro",
			AZ:           "eu-west-1a",
		}
	}
	// A side panel is 30-60 cells, never the 120 an exact-string test tends to assume.
	const panelWidth = 40
	weights := presentation.InstanceWeights()
	b.ReportAllocs()

	for b.Loop() {
		table := make([][]utils.Cell, len(instances))
		for i, instance := range instances {
			table[i] = presentation.GetInstanceDisplayCells(instance)
		}
		if _, err := utils.RenderTableFit(table, panelWidth, weights); err != nil {
			b.Fatalf("RenderTableFit: %v", err)
		}
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
