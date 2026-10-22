package agent

import (
	"context"
	"slices"
	"testing"
)

func TestBindTaskImagesAddsWhatTheBinderNamesToTheCandidates(t *testing.T) {
	ctx := WithSubagentImageCandidates(context.Background(), []string{"data:image/png;base64,AAAA"})
	ctx = WithTaskImageBinder(ctx, func(ctx context.Context, task string) context.Context {
		if task != "look at pic" {
			t.Fatalf("binder was given %q", task)
		}
		return WithSubagentImageCandidates(ctx, append(SubagentImageCandidates(ctx), "data:image/png;base64,BBBB"))
	})
	got := SubagentImageCandidates(BindTaskImages(ctx, "look at pic"))
	if !slices.Equal(got, []string{"data:image/png;base64,AAAA", "data:image/png;base64,BBBB"}) {
		t.Fatalf("candidates = %v", got)
	}
}

func TestBindTaskImagesWithoutABinderLeavesTheContextAlone(t *testing.T) {
	ctx := WithSubagentImageCandidates(context.Background(), []string{"data:image/png;base64,AAAA"})
	if got := BindTaskImages(ctx, "anything"); !slices.Equal(SubagentImageCandidates(got), []string{"data:image/png;base64,AAAA"}) {
		t.Fatalf("candidates changed without a binder: %v", SubagentImageCandidates(got))
	}
}
