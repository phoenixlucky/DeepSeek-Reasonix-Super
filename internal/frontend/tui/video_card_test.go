package tui

import (
	"strings"
	"testing"

	"reasonix/internal/contract/eventwire"
)

// The terminal says what the window says: which video went to which model, and
// how big and how long it was.
func TestSettledToolCallNamesTheVideoItHandedToTheModel(t *testing.T) {
	tool := &eventwire.Tool{ID: "v1", Name: "read_video", Output: "video clip.mp4: 4194304 bytes",
		Videos: []eventwire.VideoShown{{Name: "clip.mp4", Size: 4 << 20, DurationMs: 12000, Model: "qwen-vl"}}}
	out := strings.Join(plain(&Item{Kind: ItemTool, Tool: tool}, 100), "\n")
	for _, want := range []string{"clip.mp4", "4.0 MB", "12.0s", "qwen-vl"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the card does not say %q:\n%s", want, out)
		}
	}
}

func TestToolCallWithoutVideoSaysNothingAboutOne(t *testing.T) {
	tool := &eventwire.Tool{ID: "v2", Name: "read_video", Output: "frames"}
	if out := strings.Join(plain(&Item{Kind: ItemTool, Tool: tool}, 100), "\n"); strings.Contains(out, "video file") {
		t.Fatalf("a call that sent no video mentions one:\n%s", out)
	}
}
