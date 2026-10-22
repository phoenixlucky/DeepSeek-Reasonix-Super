package cli

import (
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/platform/feedback"
)

// A terminal session answers /feedback from the moment it starts, not only after
// something rebuilds the runtime: the surface a report names has to be in the
// first build's options.
func TestTheTerminalsFirstBuildNamesItsFeedbackSurface(t *testing.T) {
	opts := cliProfileBuildOptions("m", 0, false, event.Discard, "", cliBuildOverrides{})
	if opts.FeedbackSurface != feedback.SurfaceTUI {
		t.Fatalf("FeedbackSurface = %q, want %q", opts.FeedbackSurface, feedback.SurfaceTUI)
	}
}
