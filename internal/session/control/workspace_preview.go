package control

import (
	"context"

	"reasonix/internal/base/workspacefile"
)

func (c *Controller) previewFiles() (workspacefile.Service, error) {
	extra, err := c.ReadableExtraRoots()
	return workspacefile.Service{Primary: c.WorkspaceRoot(), Extra: extra}, err
}

func (c *Controller) WorkspacePreviewInfo(ctx context.Context, ref workspacefile.Ref) (workspacefile.Info, error) {
	files, err := c.previewFiles()
	if err != nil {
		return workspacefile.Info{}, err
	}
	return files.Info(ctx, ref)
}

func (c *Controller) WorkspacePreviewContent(ctx context.Context, ref workspacefile.Ref) ([]byte, workspacefile.Info, error) {
	files, err := c.previewFiles()
	if err != nil {
		return nil, workspacefile.Info{}, err
	}
	return files.Content(ctx, ref)
}
