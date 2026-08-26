package presentation

import (
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

func cellTexts(cells []utils.Cell) []string {
	out := make([]string, len(cells))
	for i, cell := range cells {
		out[i] = cell.Text
	}
	return out
}

func wantCells(t *testing.T, got []utils.Cell, want []utils.Cell) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got %d cells %q, want %d", len(got), cellTexts(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cell %d = %+v, want %+v (row %q)", i, got[i], want[i], cellTexts(got))
		}
	}
}

func TestGetECSClusterDisplayCells(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cluster *aws.ECSCluster
		want    []utils.Cell
	}{
		{
			"pending tasks read as a rollout in progress",
			&aws.ECSCluster{Name: "prod", Status: "ACTIVE", RunningTasksCount: 3, PendingTasksCount: 1, ActiveServicesCount: 2},
			[]utils.Cell{
				{Text: "▶", Color: color.FgGreen},
				{Text: "prod"},
				{Text: "2 services", Color: color.FgYellow},
				{Text: "3 running / 1 pending"},
				{Text: "● deploying", Color: color.FgYellow},
			},
		},
		{
			"everything up and nothing pending is healthy",
			&aws.ECSCluster{Name: "prod", Status: "ACTIVE", RunningTasksCount: 3, ActiveServicesCount: 2},
			[]utils.Cell{
				{Text: "▶", Color: color.FgGreen},
				{Text: "prod"},
				{Text: "2 services", Color: color.FgYellow},
				{Text: "3 running / 0 pending"},
				{Text: "● healthy", Color: color.FgGreen},
			},
		},
		{
			"an empty active cluster is still healthy",
			&aws.ECSCluster{Name: "empty", Status: "ACTIVE"},
			[]utils.Cell{
				{Text: "▶", Color: color.FgGreen},
				{Text: "empty"},
				{Text: "0 services", Color: color.FgYellow},
				{Text: "0 running / 0 pending"},
				{Text: "● healthy", Color: color.FgGreen},
			},
		},
		{
			"a non-active cluster shows its own status word",
			&aws.ECSCluster{Name: "old", Status: "INACTIVE", ActiveServicesCount: 1},
			[]utils.Cell{
				{Text: "⨯", Color: color.FgRed},
				{Text: "old"},
				{Text: "1 services", Color: color.FgYellow},
				{Text: "0 running / 0 pending"},
				{Text: "● INACTIVE", Color: color.FgRed},
			},
		},
		{
			"tasks draining out of a dying cluster are not a deployment",
			&aws.ECSCluster{Name: "old", Status: "DEPROVISIONING", PendingTasksCount: 2},
			[]utils.Cell{
				{Text: "?", Color: color.FgWhite},
				{Text: "old"},
				{Text: "0 services", Color: color.FgYellow},
				{Text: "0 running / 2 pending"},
				{Text: "● DEPROVISIONING", Color: color.FgRed},
			},
		},
		{
			"a status AWS did not return is named, not left as a bare bullet",
			&aws.ECSCluster{Name: "mystery"},
			[]utils.Cell{
				{Text: "?", Color: color.FgWhite},
				{Text: "mystery"},
				{Text: "0 services", Color: color.FgYellow},
				{Text: "0 running / 0 pending"},
				{Text: "● unknown", Color: color.FgRed},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantCells(t, GetECSClusterDisplayCells(tt.cluster), tt.want)
		})
	}
}

func TestGetECSServiceDisplayCells(t *testing.T) {
	s := &aws.ECSService{Name: "web", Status: "ACTIVE", LaunchType: "FARGATE", DesiredCount: 2, RunningCount: 2}

	wantCells(t, GetECSServiceDisplayCells(s), []utils.Cell{
		{Text: "▶", Color: color.FgGreen},
		{Text: "web"},
		{Text: "FARGATE", Color: color.FgYellow},
		{Text: "2/2"},
	})
}

func TestGetECSTaskDisplayCells(t *testing.T) {
	tsk := &aws.ECSTask{ID: "abc123", Status: "RUNNING", LaunchType: "FARGATE"}

	wantCells(t, GetECSTaskDisplayCells(tsk), []utils.Cell{
		{Text: "▶", Color: color.FgGreen},
		{Text: "abc123", Color: color.FgMagenta},
		{Text: "FARGATE"},
	})
}

// The cluster inspector still lays its services table out with RenderTable, so the string form has to keep carrying the colour the cells describe.
func TestGetECSServiceDisplayStringsColoursTheCells(t *testing.T) {
	forceColor(t)
	s := &aws.ECSService{Name: "web", Status: "ACTIVE", LaunchType: "FARGATE", DesiredCount: 2, RunningCount: 2}

	got := GetECSServiceDisplayStrings(s)

	want := []string{
		utils.ColoredString("▶", color.FgGreen),
		"web",
		utils.ColoredString("FARGATE", color.FgYellow),
		"2/2",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d cells %q, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cell %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// Every weight table must match the row it lays out, or RenderTableFit rejects the whole table at render time.
func TestECSWeightsMatchTheirRowWidths(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cells   int
		weights []int
	}{
		{"cluster", len(GetECSClusterDisplayCells(&aws.ECSCluster{})), ECSClusterWeights()},
		{"service", len(GetECSServiceDisplayCells(&aws.ECSService{})), ECSServiceWeights()},
		{"task", len(GetECSTaskDisplayCells(&aws.ECSTask{})), ECSTaskWeights()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.weights) != tt.cells {
				t.Errorf("%d weights for %d cells", len(tt.weights), tt.cells)
			}
		})
	}
}

// A side panel is routinely narrower than the five columns this row wants, and the name is the one thing that makes the row identifiable.
// Degrading right to left is safe here: the leftmost cell is the status icon, so dropping the badge does not drop the status.
func TestClusterRowKeepsTheNameInANarrowPanel(t *testing.T) {
	c := &aws.ECSCluster{Name: "production-eu-west-1-primary", Status: "ACTIVE", RunningTasksCount: 12, PendingTasksCount: 1, ActiveServicesCount: 9}

	for _, width := range []int{30, 40, 50, 60, 80} {
		rendered, err := utils.RenderTableFit([][]utils.Cell{GetECSClusterDisplayCells(c)}, width, ECSClusterWeights())
		if err != nil {
			t.Fatalf("width %d: %v", width, err)
		}
		if got := runewidth.StringWidth(rendered); got > width {
			t.Errorf("width %d: row is %d cells wide: %q", width, got, rendered)
		}
		if !strings.HasPrefix(rendered, "▶ product") {
			t.Errorf("width %d: row = %q, want the cluster name after the status icon", width, rendered)
		}
	}
}
