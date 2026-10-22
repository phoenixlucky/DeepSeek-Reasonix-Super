package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

// The recovery restores the retired copy under its OLD name at its old revision
// before the caller's retry re-applies the rename. That is what lets an
// ExpectedRevision from before the rename still name the fact, and what keeps
// the recovered content the old bytes rather than the staged ones.
func TestInterruptedRenameRestoresTheOldRevisionBeforeTheRetry(t *testing.T) {
	store := Store{Dir: testenv.TempDir(t)}
	first, err := store.SaveWithOptions(Memory{Name: "alpha", Description: "first", Body: "v1"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	intent := Memory{ID: first.Memory.ID, Name: "beta", Description: "renamed", Body: "v2"}
	opts := SaveOptions{ExpectedRevision: 1, RequireExpectedRevision: true}
	if killed := killDuring(t, isIndexPublish, func() { _, _ = store.SaveWithOptions(intent, opts) }); !killed {
		t.Fatal("the probe never reached the index publication, so the window was not exercised")
	}

	got, path, ok, err := store.recoverInterruptedRename(first.Memory.ID)
	if err != nil || !ok {
		t.Fatalf("the recovery did not find the interrupted rename: ok=%v err=%v", ok, err)
	}
	if got.Name != "alpha" || got.Revision != 1 {
		t.Fatalf("the recovery published the staged copy instead of restoring the old revision: name=%q revision=%d", got.Name, got.Revision)
	}
	if base := filepath.Base(path); base != "alpha.md" {
		t.Fatalf("the recovered fact is not under its old name: %s", base)
	}
	raw, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), "v1") {
		t.Fatalf("the recovered bytes are not the retired v1 body: %q err=%v", raw, err)
	}
}
