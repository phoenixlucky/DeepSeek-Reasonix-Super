package schedulerun

import (
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/tool"
	"reasonix/internal/state/schedule"
)

func TestScheduleOverAGoneWorkspaceReportsTheSharedRootMissingCause(t *testing.T) {
	gone := filepath.Join(testenv.TempDir(t), "project")
	run := &scheduleRun{sc: schedule.Schedule{Target: schedule.Target{Workspace: gone}}}
	if _, code := run.workspace(); code != tool.CodeRootMissing {
		t.Fatalf("a schedule whose workspace is gone reported %q, want the one shared cause %q", code, tool.CodeRootMissing)
	}
}
