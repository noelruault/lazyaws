package utils

import (
	"strconv"
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

const (
	benchRows       = 100
	benchPanelWidth = 60
)

// benchTable is a side panel's row shape: a name, a status, two identifiers and an age, with the status coloured.
// One row is deliberately wider than the panel, because a table that fits needs no truncation and truncation is where the work is.
func benchTable() [][]Cell {
	rows := make([][]Cell, benchRows)
	for i := range rows {
		id := strconv.Itoa(i)
		rows[i] = []Cell{
			{Text: "service-with-a-fairly-long-name-" + id},
			{Text: "✔ running", Color: color.FgGreen},
			{Text: "i-0abcdef123456789" + id},
			{Text: "eu-west-1a"},
			{Text: "12d"},
		}
	}

	return rows
}

func benchStringTable() [][]string {
	cells := benchTable()
	rows := make([][]string, len(cells))
	for i, row := range cells {
		rows[i] = make([]string, len(row))
		for j, cell := range row {
			rows[i][j] = cell.Rendered()
		}
	}

	return rows
}

// Every list rerender pays this once, and a rerender happens on every refresh tick of the focused panel.
func BenchmarkRenderTableFit(b *testing.B) {
	benchForceColor(b)
	rows := benchTable()
	weights := []int{1, 0, 0, 0, 0}
	b.ReportAllocs()

	for b.Loop() {
		if _, err := RenderTableFit(rows, benchPanelWidth, weights); err != nil {
			b.Fatalf("RenderTableFit: %v", err)
		}
	}
}

// RenderTable is the renderer the fit table replaced, kept here as the ratio to judge the fit table's per-column arithmetic against: alone, a number of microseconds says nothing.
func BenchmarkRenderTable(b *testing.B) {
	benchForceColor(b)
	rows := benchStringTable()
	b.ReportAllocs()

	for b.Loop() {
		if _, err := RenderTable(rows); err != nil {
			b.Fatalf("RenderTable: %v", err)
		}
	}
}
