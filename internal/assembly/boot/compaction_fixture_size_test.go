package boot

import (
	"strings"
	"testing"

	"reasonix/internal/contract/provider"
)

// requestChars counts what the prompt estimator reads off a request: message
// text, and each tool's name, description and parameter schema. A compaction
// fixture pads its first request to a fixed count, so the turn a threshold
// falls on belongs to the fixture rather than to the host's tool surface or
// the base prompt's length.
func requestChars(req provider.Request) int {
	n := 0
	for _, m := range req.Messages {
		n += len(m.Content)
	}
	for _, tl := range req.Tools {
		n += len(tl.Name) + len(tl.Description) + len(tl.Parameters)
	}
	return n
}

// promptPadding is the system-prompt filler that brings a fixture whose first
// request measured first up to target characters.
func promptPadding(t *testing.T, first provider.Request, target int) string {
	t.Helper()
	short := target - requestChars(first)
	if short < 0 {
		t.Fatalf("the fixture's first request is %d characters, past its %d target: raise the target, then re-check the bounds it was set for", requestChars(first), target)
	}
	return strings.Repeat("p", short)
}
