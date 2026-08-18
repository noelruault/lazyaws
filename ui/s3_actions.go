package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/resources"
)

func (gui *Gui) S3Actions() []resources.Action {
	bucket, err := gui.Panels.S3.GetSelectedItem()
	if err != nil {
		return nil
	}

	return []resources.Action{{
		Name:    "Abort stuck multipart uploads",
		Mutates: true,
		Run:     gui.s3AbortMultipartUploads(bucket),
	}}
}

// s3AbortMultipartUploads exposes one abort per upload because only the user can judge which are stuck.
func (gui *Gui) s3AbortMultipartUploads(bucket *aws.Bucket) func(context.Context, string) error {
	return func(ctx context.Context, _ string) error {
		gen := gui.Gen

		uploads, err := gui.Client.ListMultipartUploads(ctx, bucket.Name)
		if gen != gui.Gen {
			return nil // a profile switch superseded this while it was in flight
		}
		if err != nil {
			return err
		}
		if len(uploads) == 0 {
			return gui.createErrorPanel("No in-progress multipart uploads")
		}

		actions := make([]resources.Action, len(uploads))
		for i, upload := range uploads {
			actions[i] = resources.Action{
				Name:         fmt.Sprintf("%s (initiated %s)", upload.Key, upload.Initiated),
				Mutates:      true,
				Confirm:      resources.ConfirmSimple,
				Confirmation: fmt.Sprintf("Abort upload of %s?", upload.Key),
				Timeout:      10 * time.Second,
				Run: func(ctx context.Context, _ string) error {
					return gui.Client.AbortMultipartUpload(ctx, bucket.Name, upload.Key, upload.UploadID)
				},
			}
		}

		gui.g.Update(func(g *gocui.Gui) error {
			return gui.Menu(CreateMenuOptions{Title: "Multipart uploads", Items: gui.actionMenuItems(actions)})
		})

		return nil
	}
}
