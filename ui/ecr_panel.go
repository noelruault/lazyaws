package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/presentation"
	"github.com/noelruault/lazyaws/ui/tasks"
	"github.com/noelruault/lazyaws/ui/utils"
)

func (gui *Gui) getECRPanel() *panels.SideListPanel[*aws.ECRRepository] {
	return &panels.SideListPanel[*aws.ECRRepository]{
		ContextState: &panels.ContextState[*aws.ECRRepository]{
			GetMainTabs: func() []panels.MainTab[*aws.ECRRepository] {
				return []panels.MainTab[*aws.ECRRepository]{
					staticOverviewTab(gui, func(r *aws.ECRRepository) string { return "ecr-" + r.Name }, gui.ecrRepositoryOverview),
					{Key: "images", Title: "Images", Render: gui.renderECRImages},
					{Key: "scan", Title: "Scan", Render: gui.renderECRScan},
					// Policies carries the two full JSON documents; the Overview reports only their presence, and every other Config field lives there already.
					{Key: "policies", Title: "Policies", Render: gui.renderECRPolicies},
				}
			},
			GetItemContextCacheKey: func(r *aws.ECRRepository) string {
				return "ecr-" + r.Name
			},
		},

		ListPanel: panels.ListPanel[*aws.ECRRepository]{
			List: panels.NewFilteredList[*aws.ECRRepository](),
			View: gui.Views.ECR,
		},
		NoItemsMessage: "no ECR repositories",
		Gui:            gui.intoInterface(),

		Sort: func(a, b *aws.ECRRepository) bool {
			return a.Name < b.Name
		},
		GetTableCellsFit: func(r *aws.ECRRepository) []utils.Cell {
			return presentation.GetECRRepositoryDisplayCells(r)
		},
		Weights:   func(*aws.ECRRepository) []int { return presentation.ECRRepositoryWeights() },
		CopyValue: func(r *aws.ECRRepository) string { return arnOrName(r.Arn, r.Name) },
	}
}

func (gui *Gui) loadECRList() error {
	client := gui.awsClient()
	if client == nil {
		return nil
	}

	gen := gui.Generation()

	return gui.WithWaitingStatus("loading ecr", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		repos, err := client.ListECRRepositoriesDetailed(ctx)
		if err != nil {
			return err
		}

		rows := make([]*aws.ECRRepository, len(repos))
		for i := range repos {
			rows[i] = &repos[i]
		}
		swapPanelItems(gui, gen, gui.Panels.ECR, rows, ecrSelectionKey)

		return nil
	})
}

// ecrSelectionKey identifies a repository across reloads; repository names are unique per registry.
func ecrSelectionKey(repo *aws.ECRRepository) string { return repo.Name }

// ecrRepositoryOverview reads the repository off the list row and fetches what the row does not carry: the images, and the two policy documents for THIS repository.
// The image list is what keeps this off the refresh ticker: DescribeImages pages the whole repository, so its cost grows with the repository rather than staying flat. The policies are memoised in the client, so a redraw does not re-read a document that changes on a deploy.
func (gui *Gui) ecrRepositoryOverview(ctx context.Context, repo *aws.ECRRepository, width int) string {
	client := gui.awsClient()
	if client == nil {
		return overviewUnavailable("repository")
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	images, err := client.ListECRImages(fetchCtx, repo.Name)
	// The policies are read for THIS repository, not for the list: filling them per row cost two extra calls for every repository in the registry, on one deadline, and a registry big enough to run that deadline out failed the whole panel.
	policies, _ := client.GetECRRepositoryPolicies(fetchCtx, repo.Name, gui.metricsMaxAge())
	gui.throttles.observe(ecrOverviewErrs(policies, err)...)

	return presentation.FormatECRRepositoryOverview(repo, policies, images, err, width, time.Now())
}

// ecrOverviewErrs is everything one repository Overview can be throttled on, which is not the same as everything that can fail it: a policy read failing leaves the rest of the pane renderable, so a throttle on either would otherwise never reach the backoff engine and this pane would keep asking at full rate.
func ecrOverviewErrs(policies *aws.ECRRepositoryPolicies, err error) []error {
	return append([]error{err}, policies.Errs()...)
}

// renderECRPolicies fetches the two documents for the selected repository, which is where the cost belongs: they are the whole content of this tab and nothing else on screen needs them.
// The client memoises them, so switching between this tab and the Overview does not re-ask ECR for a document that changes on a deploy.
func (gui *Gui) renderECRPolicies(repo *aws.ECRRepository) tasks.TaskFunc {
	name := repo.Name

	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		client := gui.awsClient()
		if client == nil {
			gui.RenderStringMain(overviewUnavailable("policies"))
			return
		}

		gen := gui.Generation()
		fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		policies, err := client.GetECRRepositoryPolicies(fetchCtx, name, gui.metricsMaxAge())
		if gen != gui.Generation() {
			return
		}
		if err != nil {
			gui.RenderStringMain("error loading policies: " + err.Error())
			return
		}
		gui.throttles.observe(policies.Errs()...)

		gui.RenderStringMain(formatECRPolicies(policies))
	}})
}

func formatECRPolicies(policies *aws.ECRRepositoryPolicies) string {
	if policies == nil {
		return "policies not read"
	}

	out := "Repository Policy:\n"
	switch {
	case policies.PolicyErr != nil:
		out += "unavailable: " + policies.PolicyErr.Error() + "\n"
	case policies.Policy == "":
		out += "not configured\n"
	default:
		out += policies.Policy + "\n"
	}

	out += "\nLifecycle Policy:\n"
	switch {
	case policies.LifecycleErr != nil:
		out += "unavailable: " + policies.LifecycleErr.Error() + "\n"
	case policies.Lifecycle == "":
		out += "not configured\n"
	default:
		out += policies.Lifecycle + "\n"
		if policies.LifecycleEvaluated != nil {
			out += fmt.Sprintf("last evaluated: %s\n", policies.LifecycleEvaluated.Format(time.RFC3339))
		}
	}

	return out
}

func (gui *Gui) renderECRImages(repo *aws.ECRRepository) tasks.TaskFunc {
	name := repo.Name
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Generation()
		fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		images, err := gui.awsClient().ListECRImages(fetchCtx, name)
		if gen != gui.Generation() {
			return
		}
		if err != nil {
			gui.RenderStringMain("error loading images: " + err.Error())
			return
		}
		gui.RenderStringMain(formatECRImages(images))
	}})
}

func formatECRImages(images []aws.ECRImage) string {
	if len(images) == 0 {
		return "no images\n"
	}

	out := fmt.Sprintf("%d image(s):\n\n", len(images))
	for _, img := range images {
		tag := "(untagged)"
		if len(img.Tags) > 0 {
			tag = strings.Join(img.Tags, ", ")
		}
		pushed := "-"
		if img.PushedAt != nil {
			pushed = img.PushedAt.Format(time.RFC3339)
		}
		out += fmt.Sprintf("%s  %s  %s  %s\n", tag, shortDigest(img.Digest), formatByteCount(float64(img.SizeBytes)), pushed)
	}
	return out
}

// shortDigest is presentation.ShortDigest under this package's older name, kept so the four call sites here read as they did.
func shortDigest(digest string) string {
	return presentation.ShortDigest(digest)
}

// renderECRScan relies on newest-tagged-first ordering because ECR scans require a tag.
func (gui *Gui) renderECRScan(repo *aws.ECRRepository) tasks.TaskFunc {
	name := repo.Name
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		client := gui.awsClient()
		gen := gui.Generation()
		fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		images, err := client.ListECRImages(fetchCtx, name)
		if gen != gui.Generation() {
			return
		}
		if err != nil {
			gui.RenderStringMain("error loading images: " + err.Error())
			return
		}

		digest := firstTaggedImageDigest(images)
		if digest == "" {
			gui.RenderStringMain("no tagged images to scan\n")
			return
		}

		scan, err := client.GetECRImageScan(fetchCtx, name, digest)
		if gen != gui.Generation() {
			return
		}
		if err != nil {
			gui.RenderStringMain("error loading scan: " + err.Error())
			return
		}
		gui.RenderStringMain(formatECRScan(scan))
	}})
}

func firstTaggedImageDigest(images []aws.ECRImage) string {
	for _, img := range images {
		if len(img.Tags) > 0 {
			return img.Digest
		}
	}
	return ""
}

func formatECRScan(scan *aws.ECRScanResult) string {
	out := fmt.Sprintf("Status: %s\n", scan.Status)
	if scan.Description != "" {
		out += scan.Description + "\n"
	}
	if scan.CompletedAt != nil {
		out += fmt.Sprintf("Completed: %s\n", scan.CompletedAt.Format(time.RFC3339))
	}

	if len(scan.SeverityCount) > 0 {
		out += "\nSeverity counts:\n"
		for _, sev := range []string{"CRITICAL", "HIGH", "MEDIUM", "LOW", "INFORMATIONAL", "UNDEFINED"} {
			if count, ok := scan.SeverityCount[sev]; ok {
				out += fmt.Sprintf("  %s: %d\n", sev, count)
			}
		}
	}

	// Inspector's enhanced findings win; legacy basic-scan findings are the fallback when enhanced scanning isn't on.
	if len(scan.EnhancedFindings) > 0 {
		out += "\nFindings (Inspector):\n"
		for _, f := range scan.EnhancedFindings {
			out += fmt.Sprintf("\n[%s] %s (CVSS %.1f)\n", f.Severity, f.Title, f.CVSSScore)
			out += fmt.Sprintf("  Fixable: %s\n", fixableLabel(f.FixAvailable))
			if len(f.VulnerablePackages) > 0 {
				out += fmt.Sprintf("  Packages: %s\n", strings.Join(f.VulnerablePackages, ", "))
			}
		}
		return out
	}

	if len(scan.Findings) == 0 {
		out += "\nno findings\n"
		return out
	}

	out += "\nFindings:\n"
	for _, f := range scan.Findings {
		out += fmt.Sprintf("\n[%s] %s\n", f.Severity, f.Name)
		if f.Description != "" {
			out += fmt.Sprintf("  %s\n", f.Description)
		}
		if f.URI != "" {
			out += fmt.Sprintf("  %s\n", f.URI)
		}
	}
	return out
}

func fixableLabel(fixAvailable string) string {
	switch fixAvailable {
	case "YES":
		return "yes"
	case "PARTIAL":
		return "partially"
	case "NO":
		return "no"
	default:
		return "unknown"
	}
}
