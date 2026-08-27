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
					staticOverviewTab(gui, gui.ecrRepositoryOverview),
					{Key: "config", Title: "Config", Render: gui.renderECRConfig},
					{Key: "images", Title: "Images", Render: gui.renderECRImages},
					{Key: "scan", Title: "Scan", Render: gui.renderECRScan},
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
	if gui.Client == nil {
		return nil
	}

	gen := gui.Gen

	return gui.WithWaitingStatus("loading ecr", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		repos, err := gui.Client.ListECRRepositoriesDetailed(ctx)
		if err != nil {
			return err
		}
		if gen != gui.Gen {
			return nil
		}

		rows := make([]*aws.ECRRepository, len(repos))
		for i := range repos {
			rows[i] = &repos[i]
		}
		gui.Panels.ECR.SetItemsKeepSelection(rows, ecrSelectionKey)
		return gui.Panels.ECR.RerenderList()
	})
}

// ecrSelectionKey identifies a repository across reloads; repository names are unique per registry.
func ecrSelectionKey(repo *aws.ECRRepository) string { return repo.Name }

// ecrRepositoryOverview reads the repository off the list row and fetches only the images, which is the one thing the row does not carry.
// The image list is what keeps this off the refresh ticker: DescribeImages pages the whole repository, so its cost grows with the repository rather than staying flat.
func (gui *Gui) ecrRepositoryOverview(ctx context.Context, repo *aws.ECRRepository, width int) string {
	if gui.Client == nil {
		return overviewUnavailable("repository")
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	images, err := gui.Client.ListECRImages(fetchCtx, repo.Name)
	gui.throttles.observe(ecrOverviewErrs(repo, err)...)

	return presentation.FormatECRRepositoryOverview(repo, images, err, width, time.Now())
}

// ecrOverviewErrs is everything one repository Overview can be throttled on, which is not the same as everything that can fail it: the two policy reads happen per repository inside the list fetch and do not surface as its error, so a throttle on either would otherwise never reach the backoff engine and this pane would keep asking at full rate.
func ecrOverviewErrs(repo *aws.ECRRepository, err error) []error {
	return []error{err, repo.PolicyErr, repo.LifecyclePolicyErr}
}

// renderECRConfig reuses policy data already fetched with the repository row.
func (gui *Gui) renderECRConfig(repo *aws.ECRRepository) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string {
		return formatECRConfig(repo)
	})
}

func formatECRConfig(repo *aws.ECRRepository) string {
	created := "-"
	if repo.CreatedAt != nil {
		created = repo.CreatedAt.Format(time.RFC3339)
	}
	out := utils.FormatMap(0, map[string]string{
		"Name":           repo.Name,
		"URI":            repo.URI,
		"ARN":            repo.Arn,
		"Registry ID":    repo.RegistryID,
		"Created":        created,
		"Tag Mutability": repo.TagMutability,
		"Scan on push":   fmt.Sprintf("%v", repo.ScanOnPush),
		"Encryption":     formatECREncryption(repo),
	})

	out += "\nRepository Policy:\n"
	switch {
	case repo.PolicyErr != nil:
		out += "unavailable: " + repo.PolicyErr.Error() + "\n"
	case repo.PolicyText == "":
		out += "not configured\n"
	default:
		out += repo.PolicyText + "\n"
	}

	out += "\nLifecycle Policy:\n"
	switch {
	case repo.LifecyclePolicyErr != nil:
		out += "unavailable: " + repo.LifecyclePolicyErr.Error() + "\n"
	case repo.LifecyclePolicy == "":
		out += "not configured\n"
	default:
		out += repo.LifecyclePolicy + "\n"
		if repo.LifecycleEvaluated != nil {
			out += fmt.Sprintf("last evaluated: %s\n", repo.LifecycleEvaluated.Format(time.RFC3339))
		}
	}

	return out
}

func formatECREncryption(repo *aws.ECRRepository) string {
	if repo.EncryptionType == "" {
		return "-"
	}
	if repo.KMSKey != "" {
		return fmt.Sprintf("%s (%s)", repo.EncryptionType, repo.KMSKey)
	}
	return repo.EncryptionType
}

func (gui *Gui) renderECRImages(repo *aws.ECRRepository) tasks.TaskFunc {
	name := repo.Name
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		images, err := gui.Client.ListECRImages(fetchCtx, name)
		if gen != gui.Gen {
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
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		images, err := gui.Client.ListECRImages(fetchCtx, name)
		if gen != gui.Gen {
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

		scan, err := gui.Client.GetECRImageScan(fetchCtx, name, digest)
		if gen != gui.Gen {
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
