package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/resources"
)

// ECRActions labels toggles from current repository state so their direction is explicit.
func (gui *Gui) ECRActions() []resources.Action {
	repo, err := gui.Panels.ECR.GetSelectedItem()
	if err != nil {
		return nil
	}

	immutable := repo.TagMutability != "IMMUTABLE"
	scanOnPush := !repo.ScanOnPush

	return []resources.Action{
		{
			Name:         ecrToggleLabel(immutable, "tag immutability"),
			Mutates:      true,
			Confirm:      resources.ConfirmSimple,
			Confirmation: ecrToggleLabel(immutable, "tag immutability") + " for " + repo.Name + "?",
			Run: func(ctx context.Context, _ string) error {
				return gui.Client.SetImageTagMutability(ctx, repo.Name, immutable)
			},
		},
		{
			Name:         ecrToggleLabel(scanOnPush, "scan-on-push"),
			Mutates:      true,
			Confirm:      resources.ConfirmSimple,
			Confirmation: ecrToggleLabel(scanOnPush, "scan-on-push") + " for " + repo.Name + "?",
			Run:          func(ctx context.Context, _ string) error { return gui.Client.SetScanOnPush(ctx, repo.Name, scanOnPush) },
		},
		{
			Name:    "Edit lifecycle policy",
			Mutates: true,
			Prompt:  "Lifecycle policy JSON for " + repo.Name,
			// Prompt panels cannot pre-fill the current policy, so this action accepts a replacement only.
			Confirm:      resources.ConfirmSimple,
			Confirmation: "Apply this lifecycle policy to " + repo.Name + "?",
			Run: func(ctx context.Context, policyText string) error {
				if policyText == "" {
					return nil
				}
				return gui.Client.PutECRLifecyclePolicy(ctx, repo.Name, policyText)
			},
		},
		{
			// Preview remains available in read-only mode because ECR's dry run does not change the policy.
			Name:    "Preview lifecycle policy (dry run)",
			Timeout: 30 * time.Second,
			Run:     gui.ecrPreviewLifecyclePolicy(repo),
		},
		{Name: "Delete image", Mutates: true, Run: gui.ecrDeleteImage(repo)},
		{
			// Dangerous confirmation covers the force needed to delete a non-empty repository.
			Name:    "Delete repository and every image in it",
			Mutates: true,
			Confirm: resources.ConfirmDangerous,
			Token:   repo.Name,
			Run:     func(ctx context.Context, _ string) error { return gui.Client.DeleteECRRepository(ctx, repo.Name, true) },
		},
	}
}

func ecrToggleLabel(turningOn bool, what string) string {
	if turningOn {
		return "Enable " + what
	}
	return "Disable " + what
}

func (gui *Gui) ecrPreviewLifecyclePolicy(repo *aws.ECRRepository) func(context.Context, string) error {
	return func(ctx context.Context, _ string) error {
		// The empty policy is also what an unreadable one leaves behind, and telling an operator a repository has no policy is how they come to write a second one over it.
		if repo.LifecyclePolicyErr != nil {
			return fmt.Errorf("%s: lifecycle policy could not be read: %w", repo.Name, repo.LifecyclePolicyErr)
		}
		if repo.LifecyclePolicy == "" {
			return fmt.Errorf("%s has no lifecycle policy to preview", repo.Name)
		}

		preview, err := gui.Client.PreviewLifecyclePolicy(ctx, repo.Name, "")
		if err != nil {
			return err
		}

		gui.g.Update(func(g *gocui.Gui) error {
			return gui.createConfirmationPanel("Lifecycle Policy Preview", formatECRLifecyclePolicyPreview(preview), nil, nil)
		})

		return nil
	}
}

func formatECRLifecyclePolicyPreview(preview *aws.ECRLifecyclePolicyPreview) string {
	out := fmt.Sprintf("Status: %s\nImages that would expire: %d\n", preview.Status, preview.ExpiringImageCount)
	if len(preview.ExpiringImages) == 0 {
		return out
	}

	out += "\n"
	for _, img := range preview.ExpiringImages {
		tag := "(untagged)"
		if len(img.Tags) > 0 {
			tag = strings.Join(img.Tags, ", ")
		}
		out += fmt.Sprintf("  %s  %s\n", tag, shortDigest(img.Digest))
	}

	return out
}

// ecrDeleteImage fetches at run time because flat repository rows omit images.
func (gui *Gui) ecrDeleteImage(repo *aws.ECRRepository) func(context.Context, string) error {
	return func(ctx context.Context, _ string) error {
		images, err := gui.Client.ListECRImages(ctx, repo.Name)
		if err != nil {
			return err
		}
		if len(images) == 0 {
			return fmt.Errorf("%s has no images", repo.Name)
		}

		actions := make([]resources.Action, len(images))
		for i, image := range images {
			actions[i] = resources.Action{
				Name:         ecrImageLabel(image),
				Mutates:      true,
				Confirm:      resources.ConfirmSimple,
				Confirmation: fmt.Sprintf("Delete %s/%s? This cannot be undone.", repo.Name, ecrImageLabel(image)),
				Run: func(ctx context.Context, _ string) error {
					return gui.Client.DeleteECRImage(ctx, repo.Name, image.Digest)
				},
			}
		}

		// Action callbacks run off the UI thread, so popup creation must be queued.
		gui.g.Update(func(g *gocui.Gui) error {
			return gui.Menu(CreateMenuOptions{Title: "Delete image — choose one", Items: gui.actionMenuItems(actions)})
		})

		return nil
	}
}

func ecrImageLabel(image aws.ECRImage) string {
	tag := "(untagged)"
	if len(image.Tags) > 0 {
		tag = strings.Join(image.Tags, ", ")
	}
	return fmt.Sprintf("%s (%s)", tag, shortDigest(image.Digest))
}
