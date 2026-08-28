package presentation

import (
	"strings"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/ui/utils"
)

// Gallery renders every shared display component once, with sample data, so the design can be judged against the real terminal render instead of an HTML approximation.
// Each block is captioned with the code symbol that draws it, which is what makes a verdict on the picture actionable ("drop X", "keep Y") without archaeology.
// Sample data is deliberately fictional and stable: the gallery is a picture of the components, never of an account.
func Gallery(width int) string {
	sections := []struct {
		symbol string
		body   string
	}{
		{"ResourceHeader + StatBoxes(compact) via mergeRightAligned", galleryHeader(width)},
		{"StatBoxes(filled) — the Health cards", galleryHealthCards(width)},
		{"BoxedTable — bordered table, faint header, rule under it", galleryServiceTable(width)},
		{"BoxedTable with a flexible last column", galleryTaskTable(width)},
		{"Badge — one colour vocabulary for every state word", galleryBadges()},
		{"Gauge — textual meter for metrics", galleryGauges()},
		{"kvBlock — aligned label/value rows", galleryKvBlock()},
		{"SectionTitle — the mockups' icon per section", galleryTitles()},
		{"tagsBody — style A: tagChips (flip tagStyleChips)", tagsBodySample(width, true)},
		{"tagsBody — style B: plain lines (current)", tagsBodySample(width, false)},
	}

	var out []string
	for _, s := range sections {
		out = append(out, utils.ColoredString("── "+s.symbol+" ", color.Faint), s.body, "")
	}

	return truncateBlock(strings.Join(out, "\n"), width)
}

func galleryHeader(width int) string {
	return mergeRightAligned(width,
		ResourceHeader("ECS Cluster", "app-cluster", Badge("healthy"), "",
			"eu-west-1",
			"1/1 services steady",
		),
		StatBoxes(0, []Stat{
			{Label: "Services", Value: utils.Cell{Text: "1 / 1", Color: color.FgGreen}},
			{Label: "Tasks", Value: utils.Cell{Text: "1 running", Color: color.FgGreen}},
			{Label: "Pending", Value: utils.Cell{Text: "0"}},
		}),
	)
}

func galleryHealthCards(width int) string {
	return StatBoxes(min(width, 90), []Stat{
		{Label: "Cluster", Value: utils.Cell{Text: "● ACTIVE", Color: color.FgGreen}},
		{Label: "Services", Value: utils.Cell{Text: "1 healthy", Color: color.FgGreen}},
		{Label: "Deployments", Value: utils.Cell{Text: "deploying", Color: color.FgYellow}},
	})
}

func galleryServiceTable(width int) string {
	return SectionTitleWithNote(min(width, 90), "Service Summary", "1 service") + "\n" +
		BoxedTable(min(width, 90), []int{1, 0, 0, 0, 0},
			[]string{"Service", "Desired", "Running", "Pending", "Status"},
			[][]utils.Cell{
				{{Text: "app-service", Color: color.Bold}, {Text: "1"}, {Text: "1", Color: color.FgGreen}, {Text: "0"}, {Text: "● steady", Color: color.FgGreen}},
				{{Text: "app-worker", Color: color.Bold}, {Text: "3"}, {Text: "1"}, {Text: "2"}, {Text: "● scaling", Color: color.FgYellow}},
			})
}

func galleryTaskTable(width int) string {
	return SectionTitle("Tasks") + "\n" +
		BoxedTable(min(width, 90), []int{0, 0, 0, 1},
			[]string{"Task", "Revision", "Status", "Image"},
			[][]utils.Cell{
				{{Text: "7c81a4b2e5f6", Color: color.Faint}, {Text: "app:42"}, {Text: "RUNNING", Color: color.FgGreen}, {Text: "app-api:v1.42.0 (+1 sidecar)"}},
				{{Text: "9d02c7a1b3e4", Color: color.Faint}, {Text: "app:41"}, {Text: "PENDING", Color: color.FgYellow}, {Text: "app-api:v1.41.9"}},
			})
}

func galleryBadges() string {
	words := []string{"running", "ACTIVE", "IN_PROGRESS", "draining", "stopped", "unhealthy", "some-new-state"}
	badges := make([]string, len(words))
	for i, w := range words {
		badges[i] = Badge(w)
	}

	return strings.Join(badges, "   ")
}

func galleryGauges() string {
	return kvBlock([]kv{
		{"CPU", Gauge(10, 26.2) + "  268 / 1024 units"},
		{"Memory", Gauge(10, 70.0) + "  1433 / 2048 MiB"},
		{"Disk", Gauge(10, 100)},
	})
}

func galleryKvBlock() string {
	return kvBlock([]kv{
		{"ARN", "arn:aws:ecs:eu-west-1:123456789012:cluster/app-cluster"},
		{"Region", "eu-west-1"},
		{"Container Insights", "disabled"},
		{"Execute command", "DEFAULT (task awslogs driver)"},
	})
}

func galleryTitles() string {
	titles := []string{"Configuration", "Network", "Metrics", "Health", "Storage", "Security", "Console", "Tags", "Services", "Capacity", "Tasks"}
	line := make([]string, len(titles))
	for i, t := range titles {
		line[i] = SectionTitle(t)
	}

	return strings.Join(line, "   ")
}

// tagsBodySample renders the tag section in one forced style, so the gallery always shows both candidates whatever tagStyleChips currently says.
func tagsBodySample(width int, chips bool) string {
	previous := tagStyleChips
	tagStyleChips = chips
	defer func() { tagStyleChips = previous }()

	return SectionTitle("Tags") + "\n" + tagsBody(min(width, 90), []kv{
		{"Environment", "staging"},
		{"Team", "security"},
		{"Owner", ""},
	})
}
