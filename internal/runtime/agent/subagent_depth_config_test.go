package agent

import "testing"

// The config value 0 removes the stop; a negative one is the single-layer
// boundary; a positive one is read as written.
func TestSubagentDepthFromConfig(t *testing.T) {
	for in, want := range map[int]int{0: UnlimitedSubagentDepth, -2: 1, 1: 1, 8: 8, 30: 30} {
		if got := SubagentDepthFromConfig(in); got != want {
			t.Errorf("max_subagent_depth %d -> %d, want %d", in, got, want)
		}
	}
	if NormalizeMaxSubagentDepth(SubagentDepthFromConfig(0)) != UnlimitedSubagentDepth {
		t.Error("normalizing the unlimited depth changed it")
	}
}
