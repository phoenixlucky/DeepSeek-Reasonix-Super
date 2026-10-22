package delegation

import (
	"testing"

	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/runtime/agent"
)

// A parent that changed depth in place has a provider built with the old one, so a
// child sharing that provider starts on the depth in force now, as it would have
// after a rebuild; a child that names its own effort keeps it.
func TestAChildSharingTheParentProviderStartsOnTheDepthInForce(t *testing.T) {
	for _, tc := range []struct {
		name, args, want string
	}{
		{"inherits", `{"prompt":"find callers"}`, "high"},
		{"names its own effort", `{"prompt":"find callers","effort":"low"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sub := &mockProvider{name: "sub", chunks: []provider.Chunk{{Type: provider.ChunkText, Text: "done"}, {Type: provider.ChunkDone}}}
			task := newTestTaskTool(t, sub, tool.NewRegistry(), "sys", "", "", nil)
			ctx := agent.WithRequestEffort(testTaskContext(), "high")
			if _, err := task.Execute(ctx, []byte(tc.args)); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got := sub.lastReq.EffortOverride; got != tc.want {
				t.Fatalf("child request carried %q, want %q", got, tc.want)
			}
		})
	}
}
