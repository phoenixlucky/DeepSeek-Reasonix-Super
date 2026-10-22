package agent

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/tool"
	"reasonix/internal/state/sessionstore"
)

// Another session opening its store sweeps the scratch spill directory and
// removes it once empty; a spill landing at that moment must still keep the
// result whole rather than fall back to truncation.
func TestScratchSpillSurvivesTheDirectoryBeingSweptAway(t *testing.T) {
	big := strings.Repeat("a line of tool output padded out a bit\n", 2000)
	sweepRoot := testenv.TempDir(t)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = sessionstore.ReconcileCleanupPending(sweepRoot, nil)
		}
	})
	defer func() { close(stop); wg.Wait() }()

	bare := New(nil, tool.NewRegistry(), sessionstore.NewSession("sys"), Options{}, event.Discard)
	for i := range 300 {
		id := fmt.Sprintf("race-%d", i)
		out, _, notice := bare.boundToolOutput(big, "bash", id, "", false)
		if notice != "" {
			t.Fatalf("round %d: the result was truncated instead of spilled: %q", i, notice)
		}
		_ = os.Remove(spillPathIn(t, out))
	}
}
