package agent

import (
	"context"
	"encoding/json"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
)

// SetNativeVideo installs the question tools ask, as they run, of whether the
// selected model watches a video file natively. It is asked per call: the model
// can change between calls.
func (a *Agent) SetNativeVideo(src tool.NativeVideoSource) { a.svc.nativeVideo = src }

// nativeVideoModel names the model a video read in this call went to.
func (a *Agent) nativeVideoModel() string {
	if a.svc.nativeVideo == nil {
		return ""
	}
	n, _ := a.svc.nativeVideo()
	return n.Model
}

// shownVideos is what a window is told about the videos a call handed over.
func shownVideos(refs []provider.VideoRef, model string) []event.VideoShown {
	if len(refs) == 0 {
		return nil
	}
	out := make([]event.VideoShown, len(refs))
	for i, r := range refs {
		out[i] = event.VideoShown{Name: r.Name, Size: r.Size, DurationMs: r.DurationMs, Model: model}
	}
	return out
}

// toolMedia is what a call showed the model beside its text.
type toolMedia struct {
	images []string
	videos []provider.VideoRef
}

// mediaTool is how a tool that can return pictures or video is run: video first,
// since a tool that has both is asking for the richer channel.
func mediaTool(t tool.Tool) (func(context.Context, json.RawMessage) (string, toolMedia, error), bool) {
	if vt, ok := t.(tool.VideoTool); ok {
		return func(ctx context.Context, args json.RawMessage) (string, toolMedia, error) {
			text, images, videos, err := vt.ExecuteWithVideo(ctx, args)
			return text, toolMedia{images: images, videos: videos}, err
		}, true
	}
	if it, ok := t.(tool.ImageTool); ok {
		return func(ctx context.Context, args json.RawMessage) (string, toolMedia, error) {
			text, images, err := it.ExecuteWithImages(ctx, args)
			return text, toolMedia{images: images}, err
		}, true
	}
	return nil, false
}
