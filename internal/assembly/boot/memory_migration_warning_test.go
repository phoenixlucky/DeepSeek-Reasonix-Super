package boot

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/state/memory"
)

// The boot warning used to be one generic line. A fact the migration could not
// read is skipped, so the warning has to name it: the operator can then fix the
// permission or inspect the file instead of guessing what was left behind.
func TestBuildMemoryMigrationWarnsWithTheSkippedFact(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 000 does not deny reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod 000 does not deny reads")
	}
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	writeFile(t, dir, "reasonix.toml", effectProbeConfig)
	approveWorkspace(t, dir)

	store := memory.StoreFor(config.MemoryUserDir(), dir)
	legacy := func(name string) string {
		return "---\nname: " + name + "\nmetadata:\n  type: project\n  scope: project\n---\n\nlegacy " + name + " body\n"
	}
	writeFile(t, store.Dir, "alpha.md", legacy("alpha"))
	writeFile(t, store.Dir, "beta.md", legacy("beta"))
	betaPath := filepath.Join(store.Dir, "beta.md")
	if err := os.Chmod(betaPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(betaPath, 0o644) })

	var warnings []event.Event
	ctrl, err := Build(context.Background(), Options{
		WorkspaceRoot: dir,
		Sink: event.FuncSink(func(e event.Event) {
			if e.Level == event.LevelWarn {
				warnings = append(warnings, e)
			}
		}),
	})
	if err != nil {
		t.Fatalf("an unreadable fact must not abort boot: %v", err)
	}
	defer ctrl.Close()

	for _, warning := range warnings {
		if strings.Contains(warning.Text, "Memory metadata migration") && strings.Contains(warning.Detail, "beta.md") {
			return
		}
	}
	t.Fatalf("no warning names the skipped fact: %+v", warnings)
}
