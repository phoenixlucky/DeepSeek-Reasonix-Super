package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/tool"
	"reasonix/internal/runtime/writeclaim"
	"reasonix/internal/safety/evidence"
)

func optimisticFixture(t *testing.T) (root string, sched *writeclaim.SubagentScheduler, bound *tool.Registry, removed []string) {
	t.Helper()
	root = testenv.TempDir(t)
	claim, err := writeclaim.OptimisticWorkspaceClaim(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	reg.Add(&recordingWriter{name: "write_file", writesPaths: true})
	reg.Add(&recordingWriter{name: "mcp__deploy__release"})
	reg.Add(&recordingWriter{name: "read_notes", readOnly: true})
	sched = writeclaim.NewSubagentScheduler(4, 2)
	bound, removed = BindWritePaths(reg, writeclaim.NewWriteGrant(claim), nil, sched, root, false)
	return root, sched, bound, removed
}

// A strict whole-workspace run loses the writers the host cannot path-scope; an
// optimistic run is the user's choice to keep them and have them observed.
func TestOptimisticRunKeepsWritersTheStrictRunLoses(t *testing.T) {
	_, _, bound, removed := optimisticFixture(t)
	if len(removed) != 0 {
		t.Fatalf("removed = %v, want nothing", removed)
	}
	for _, name := range []string{"write_file", "mcp__deploy__release", "read_notes"} {
		if _, ok := bound.Get(name); !ok {
			t.Errorf("optimistic run lost %s", name)
		}
	}
	root := testenv.TempDir(t)
	strict, _ := writeclaim.WholeWorkspaceWriteClaim(root)
	reg := tool.NewRegistry()
	reg.Add(&recordingWriter{name: "mcp__deploy__release"})
	if _, gone := BindWritePaths(reg, writeclaim.NewWriteGrant(strict), nil, writeclaim.NewSubagentScheduler(4, 2), root, false); len(gone) != 1 {
		t.Fatalf("strict run removed %v, want the MCP writer dropped as before", gone)
	}
}

func TestOptimisticWriteHoldsItsPathOnlyWhileItRuns(t *testing.T) {
	root, sched, bound, _ := optimisticFixture(t)
	writer := mustGet(t, bound, "write_file")
	target := filepath.Join(root, "a.txt")
	args, _ := json.Marshal(map[string]string{"path": target, "content": "x"})

	hold, err := sched.ReserveWrite(mustClaim(t, root, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = writer.Execute(context.Background(), args)
	if !errors.Is(err, writeclaim.ErrWritePathBusy) {
		t.Fatalf("a write to a path another run is writing: err = %v, want ErrWritePathBusy", err)
	}
	other, _ := json.Marshal(map[string]string{"path": filepath.Join(root, "b.txt"), "content": "x"})
	if _, err := writer.Execute(context.Background(), other); err != nil {
		t.Fatalf("a write to a different path was refused: %v", err)
	}
	hold()
	if _, err := writer.Execute(context.Background(), args); err != nil {
		t.Fatalf("the path stayed held after the other write ended: %v", err)
	}
}

func TestOptimisticWriteIsScopedToTheWorkspaceNotBeyondIt(t *testing.T) {
	root, _, bound, _ := optimisticFixture(t)
	writer := mustGet(t, bound, "write_file")
	outside := filepath.Join(filepath.Dir(root), "escape.txt")
	args, _ := json.Marshal(map[string]string{"path": outside, "content": "x"})
	if _, err := writer.Execute(context.Background(), args); !errors.Is(err, writeclaim.ErrWriteFenceClosed) {
		t.Fatalf("a write outside the workspace: err = %v, want the fence refusal", err)
	}
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("the write landed outside the workspace")
	}
}

func TestReceiptSaysAnOptimisticRunWasNotFenced(t *testing.T) {
	root := testenv.TempDir(t)
	summary := evidence.ChildEvidenceSummary{Receipts: []evidence.Receipt{{
		ToolName: "write_file", Write: true, Mutation: true, Success: true, Paths: []string{filepath.Join(root, "a.txt")},
	}}}
	strict, _ := writeclaim.WholeWorkspaceWriteClaim(root)
	optimistic, _ := writeclaim.OptimisticWorkspaceClaim(root)
	if got := formatHostReceipts(summary, strict); strings.Contains(got, HostReceiptsUnfencedLabel) {
		t.Fatalf("a strict run was labelled unfenced:\n%s", got)
	}
	got := formatHostReceipts(summary, optimistic)
	if !strings.Contains(got, HostReceiptsUnfencedLabel+":") || !strings.Contains(got, "changed:") {
		t.Fatalf("receipt does not say the run was not fenced:\n%s", got)
	}
}

func mustClaim(t *testing.T, root string, paths ...string) writeclaim.WritePathSet {
	t.Helper()
	c, err := writeclaim.NormalizeWritePaths(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
