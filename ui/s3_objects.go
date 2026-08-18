// Package ui keeps S3 object cursors in main because SideListPanel only owns side-view selection.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/tasks"
	"github.com/noelruault/lazyaws/ui/types"
)

type s3ObjectsState struct {
	bucket  string
	prefix  string
	objects []aws.S3Object
}

func s3ObjectsDrillDown(state s3ObjectsState, cursor int) s3ObjectsState {
	if cursor < 0 || cursor >= len(state.objects) {
		return state
	}
	row := state.objects[cursor]
	if !row.IsFolder {
		return state
	}
	return s3ObjectsState{bucket: state.bucket, prefix: row.Key}
}

func s3ObjectsDrillUp(state s3ObjectsState) s3ObjectsState {
	if state.prefix == "" {
		return state
	}
	trimmed := strings.TrimSuffix(state.prefix, "/")
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
		return s3ObjectsState{bucket: state.bucket, prefix: trimmed[:idx+1]}
	}
	return s3ObjectsState{bucket: state.bucket, prefix: ""}
}

// renderS3Objects preserves position only while the selected bucket is unchanged.
func (gui *Gui) renderS3Objects(bucket *aws.Bucket) tasks.TaskFunc {
	if gui.s3Objects.bucket != bucket.Name {
		gui.s3Objects = s3ObjectsState{bucket: bucket.Name}
	}
	return gui.NewTask(TaskOpts{Wrap: true, Func: func(ctx context.Context) {
		gui.loadS3ObjectsAndRender(ctx)
	}})
}

func (gui *Gui) loadS3ObjectsAndRender(ctx context.Context) {
	gen := gui.Gen
	bucket, prefix := gui.s3Objects.bucket, gui.s3Objects.prefix

	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := gui.Client.ListObjects(fetchCtx, bucket, prefix, nil)
	if gen != gui.Gen {
		return
	}
	if err != nil {
		gui.RenderStringMain("error loading objects: " + err.Error())
		return
	}

	gui.s3Objects.objects = result.Objects
	rows := gui.s3ObjectRows()
	gui.RenderStringMain(renderMainRows(rows, gui.mainCursor(rows)))
}

func (gui *Gui) reloadS3Objects() error {
	return gui.QueueTask(gui.NewTask(TaskOpts{Wrap: true, Func: func(ctx context.Context) {
		gui.loadS3ObjectsAndRender(ctx)
	}}))
}

// s3ObjectRows exposes the object listing as navigable rows; the cursor, marking and scrolling belong to the main panel.
func (gui *Gui) s3ObjectRows() *panels.MainRows {
	state := gui.s3Objects
	cells := make([][]string, len(state.objects))
	for i, obj := range state.objects {
		cells[i] = s3ObjectRowCells(obj, state.prefix)
	}

	rows := &panels.MainRows{
		Header:       fmt.Sprintf("s3://%s/%s", state.bucket, state.prefix),
		EmptyMessage: "(empty)",
		Cells:        cells,
		Enter: func(i int) error {
			obj := state.objects[i]
			if obj.IsFolder {
				gui.s3Objects = s3ObjectsDrillDown(gui.s3Objects, i)
				return gui.reloadS3Objects()
			}
			return gui.handleS3ObjectDownload(obj)
		},
		Actions: func(i int) error {
			obj := state.objects[i]
			if obj.IsFolder {
				return nil
			}
			return gui.s3ObjectMenu(obj)
		},
	}

	// Only offer "back" below the bucket root; at the root the key should leave the panel as it always has.
	if state.prefix != "" {
		rows.Back = func() error {
			gui.s3Objects = s3ObjectsDrillUp(gui.s3Objects)
			return gui.reloadS3Objects()
		}
	}

	return rows
}

func s3ObjectRowCells(obj aws.S3Object, prefix string) []string {
	name := strings.TrimPrefix(obj.Key, prefix)
	if obj.IsFolder {
		return []string{"[dir]", name, "-", "-", "-"}
	}
	return []string{"", name, formatByteCount(float64(obj.Size)), obj.StorageClass, obj.LastModified}
}

// s3ObjectMenu is the per-row affordance the main panel's actions key opens.
func (gui *Gui) s3ObjectMenu(obj aws.S3Object) error {
	bucket := gui.s3Objects.bucket
	items := []*types.MenuItem{
		{Label: "Versions", OnPress: func() error { return gui.handleS3ObjectVersions(bucket, obj.Key) }},
		{Label: "Presigned URL (1h)", OnPress: func() error { return gui.handleS3PresignedURL(bucket, obj.Key) }},
	}
	return gui.Menu(CreateMenuOptions{Title: "Object: " + obj.Key, Items: items})
}

func (gui *Gui) handleS3ObjectDownload(obj aws.S3Object) error {
	bucket := gui.s3Objects.bucket
	key := obj.Key
	defaultDest := key[strings.LastIndex(key, "/")+1:]

	return gui.createPromptPanel(fmt.Sprintf("Local destination for %s (default: %s)", key, defaultDest), func(g *gocui.Gui, v *gocui.View) error {
		dest := gui.trimmedContent(v)
		if dest == "" {
			dest = defaultDest
		}
		return gui.WithWaitingStatus("downloading "+key, func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			return gui.Client.DownloadObject(ctx, bucket, key, dest)
		})
	})
}

// handleS3ObjectVersions queues popup creation because waiting work runs off the UI thread.
func (gui *Gui) handleS3ObjectVersions(bucket, key string) error {
	return gui.WithWaitingStatus("loading versions", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		versions, err := gui.Client.ListObjectVersions(ctx, bucket, key)
		if err != nil {
			return err
		}

		gui.g.Update(func(g *gocui.Gui) error {
			items := make([]*types.MenuItem, 0, len(versions))
			for _, v := range versions {
				version := v
				label := fmt.Sprintf("%s  %s  %s", version.VersionId, formatByteCount(float64(version.Size)), version.LastModified)
				if version.IsLatest {
					label += " (latest)"
				}
				// Read-only mode keeps version metadata visible but replaces restore with a refusal.
				onPress := func() error { return gui.handleS3RestoreVersionConfirm(bucket, key, version) }
				if gui.readOnly() {
					onPress = func() error { return gui.refuseReadOnly("Restoring a version") }
				}
				items = append(items, &types.MenuItem{Label: label, OnPress: onPress})
			}
			return gui.Menu(CreateMenuOptions{Title: "Versions of " + key, Items: items})
		})
		return nil
	})
}

// handleS3RestoreVersionConfirm mutates views directly because menu callbacks already run on the UI thread.
func (gui *Gui) handleS3RestoreVersionConfirm(bucket, key string, version aws.S3ObjectVersion) error {
	prompt := fmt.Sprintf("Restore %s to version %s? This becomes the new current version.", key, version.VersionId)
	return gui.createConfirmationPanel("Restore version", prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus("restoring "+key, func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			return gui.Client.CopyObjectVersion(ctx, bucket, key, version.VersionId)
		})
	}, nil)
}

// handleS3PresignedURL uses a popup to avoid adding a clipboard dependency.
func (gui *Gui) handleS3PresignedURL(bucket, key string) error {
	return gui.WithWaitingStatus("generating presigned url", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		url, err := gui.Client.GeneratePresignedURL(ctx, bucket, key, 3600)
		if err != nil {
			return err
		}

		gui.g.Update(func(g *gocui.Gui) error {
			return gui.createConfirmationPanel("Presigned URL (expires in 1h)", url, func(g *gocui.Gui, v *gocui.View) error { return nil }, nil)
		})
		return nil
	})
}
