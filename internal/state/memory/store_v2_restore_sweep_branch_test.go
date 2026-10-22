package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

// The sweep's active branch must DELETE the leftover, never rename it back: a
// fact that is already published must not regain an archive entry, or
// ListArchived would report a live fact as archived again.
func TestSweepOfAnActiveRestoreDoesNotRecreateTheArchiveEntry(t *testing.T) {
	dir := testenv.TempDir(t)
	s := Store{Dir: dir}
	alpha, err := s.SaveWithOptions(Memory{Name: "alpha", Description: "published then left behind", Type: TypeProject, Body: "alpha body"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	alphaArchive, err := s.Archive("alpha")
	if err != nil {
		t.Fatal(err)
	}
	// Keep a copy of the archived bytes, then let the real restore publish alpha.
	bytes, err := os.ReadFile(alphaArchive)
	if err != nil {
		t.Fatal(err)
	}
	// Same stamp prefix as the archive entry (the helper parses it), but not .md,
	// so the copy is never read as a fact.
	copied := filepath.Join(dir, ".archive", strings.TrimSuffix(filepath.Base(alphaArchive), ".md")+".bin")
	if err := os.WriteFile(copied, bytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RestoreArchived(alphaArchive); err != nil {
		t.Fatalf("the fixture could not restore alpha: %v", err)
	}
	if _, active, err := s.Read("alpha"); err != nil || !active {
		t.Fatalf("fixture: alpha is not active after its restore: active=%v err=%v", active, err)
	}
	// Now reproduce the post-publish kill: alpha is active AND a hidden staging
	// copy carrying its id exists.
	stageArchivedEntryForTest(t, copied, alpha.Memory.ID)

	if _, err := s.Save(Memory{Name: "beta", Description: "next restore", Type: TypeProject, Body: "beta body"}); err != nil {
		t.Fatal(err)
	}
	betaArchive, err := s.Archive("beta")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Dir: dir}).RestoreArchived(betaArchive); err != nil {
		t.Fatalf("the restore that runs the sweep: %v", err)
	}

	if left := stagingEntries(t, dir); len(left) != 0 {
		t.Fatalf("the sweep left staging copies behind: %v", left)
	}
	archived, err := Store{Dir: dir}.ListArchived()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range archived {
		if entry.ID == alpha.Memory.ID {
			t.Fatalf("a live fact regained an archive entry: %+v", entry)
		}
	}
	live, ok, err := (Store{Dir: dir}).Read("alpha")
	if err != nil || !ok || live.Body != "alpha body" {
		t.Fatalf("the live fact was disturbed by the sweep: ok=%v err=%v memory=%+v", ok, err, live)
	}
}
