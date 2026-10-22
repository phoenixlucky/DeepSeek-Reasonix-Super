package memory

// An archive restore interrupted after it published must converge on the next
// restore instead of leaving one id active and archived at once. The staging
// name carries the id, so no content comparison decides a leftover's fate.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

const (
	restoreKillDirEnv     = "RX_RESTORE_KILL_DIR"
	restoreKillArchiveEnv = "RX_RESTORE_KILL_ARCHIVE"
)

// stagingEntries returns the hidden restore copies in the store's .archive.
func stagingEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, ".archive"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), restoreStagingPrefix) {
			out = append(out, entry.Name())
		}
	}
	return out
}

func onlyStagingEntry(t *testing.T, dir string) string {
	t.Helper()
	held := stagingEntries(t, dir)
	if len(held) != 1 {
		t.Fatalf("staging entries in %s = %v, want exactly one", dir, held)
	}
	return held[0]
}

func assertNoStagingListed(t *testing.T, s Store) {
	t.Helper()
	archived, err := s.ListArchived()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range archived {
		if strings.HasPrefix(filepath.Base(entry.Path), restoreStagingPrefix) {
			t.Fatalf("ListArchived reports a hidden staging copy: %+v", entry)
		}
	}
}

// stageArchivedEntryForTest emulates a process killed after staging an archive
// entry but before it published the active file.
func stageArchivedEntryForTest(t *testing.T, archivePath, id string) string {
	t.Helper()
	base := filepath.Base(archivePath)
	stamp := base[:len(restoreStampLayout)]
	staging := filepath.Join(filepath.Dir(archivePath), restoreStagingPrefix+id+"-"+stamp)
	if err := os.Rename(archivePath, staging); err != nil {
		t.Fatal(err)
	}
	return staging
}

func TestRestoreArchivedAfterAKillSweepsTheStagingCopy(t *testing.T) {
	if os.Getenv(restoreKillDirEnv) != "" {
		restoreKillChild()
		return
	}
	dir := testenv.TempDir(t)
	s := Store{Dir: dir}
	alpha, err := s.SaveWithOptions(Memory{Name: "alpha", Description: "restored then killed", Type: TypeProject, Body: "alpha body"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	alphaArchive, err := s.Archive("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Memory{Name: "beta", Description: "restored next", Type: TypeProject, Body: "beta body"}); err != nil {
		t.Fatal(err)
	}
	betaArchive, err := s.Archive("beta")
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRestoreArchivedAfterAKillSweepsTheStagingCopy$")
	cmd.Env = append(os.Environ(),
		restoreKillDirEnv+"="+dir,
		restoreKillArchiveEnv+"="+alphaArchive,
	)
	out, runErr := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != 137 {
		t.Fatalf("the child was not killed at the staging removal (run error %v, output %q)", runErr, out)
	}

	// The kill landed after publication: the active file and its index line
	// exist and the archive entry is hidden under its staging name.
	held := onlyStagingEntry(t, dir)
	if !strings.Contains(held, alpha.Memory.ID) {
		t.Fatalf("staging entry %q does not carry the restored id %q", held, alpha.Memory.ID)
	}
	fresh := Store{Dir: dir}
	if _, ok, err := fresh.Read("alpha"); err != nil || !ok {
		t.Fatalf("the killed restore left no readable active fact: ok=%v err=%v", ok, err)
	}
	assertNoStagingListed(t, fresh)

	// A restore of a DIFFERENT entry reconciles the leftover: the id is active,
	// so the hidden duplicate goes and the live fact is untouched.
	if _, err := fresh.RestoreArchived(betaArchive); err != nil {
		t.Fatalf("restore after the kill: %v", err)
	}
	if left := stagingEntries(t, dir); len(left) != 0 {
		t.Fatalf("the sweep left staging copies behind: %v", left)
	}
	active, ok, err := fresh.Read("alpha")
	if err != nil || !ok {
		t.Fatalf("the sweep deleted the live fact: ok=%v err=%v", ok, err)
	}
	if active.Body != "alpha body" {
		t.Fatalf("the live fact changed: %+v", active)
	}
	assertNoStagingListed(t, fresh)
	body := mustReadString(t, filepath.Join(dir, indexFile))
	if n := strings.Count(body, "(alpha.md)"); n != 1 {
		t.Fatalf("MEMORY.md names the live fact %d times:\n%s", n, body)
	}
}

func restoreKillChild() {
	dir := os.Getenv(restoreKillDirEnv)
	archive := os.Getenv(restoreKillArchiveEnv)
	if dir == "" || archive == "" {
		os.Exit(2)
	}
	s := Store{Dir: dir}
	memoryFault = func(op, path string) error {
		if op == "remove" && strings.HasPrefix(filepath.Base(path), restoreStagingPrefix) {
			// Termination, not an error: no return, no defer, no undo.
			os.Exit(137)
		}
		return nil
	}
	if _, err := s.RestoreArchived(archive); err != nil {
		println("restore child: returned before the staging removal:", err.Error())
		os.Exit(11)
	}
	println("restore child: completed without reaching the staging removal")
	os.Exit(12)
}

func TestRestoreArchivedSweepPutsBackAnEntryThatNeverPublished(t *testing.T) {
	dir := testenv.TempDir(t)
	s := Store{Dir: dir}
	alpha, err := s.SaveWithOptions(Memory{Name: "alpha", Description: "never published", Type: TypeProject, Body: "alpha body"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	alphaArchive, err := s.Archive("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Memory{Name: "beta", Description: "next restore", Type: TypeProject, Body: "beta body"}); err != nil {
		t.Fatal(err)
	}
	betaArchive, err := s.Archive("beta")
	if err != nil {
		t.Fatal(err)
	}
	stageArchivedEntryForTest(t, alphaArchive, alpha.Memory.ID)

	fresh := Store{Dir: dir}
	if _, active := mustStore(t, fresh).Read("alpha"); active {
		t.Fatal("fixture staged an entry whose id is still active")
	}
	assertNoStagingListed(t, fresh)

	if _, err := fresh.RestoreArchived(betaArchive); err != nil {
		t.Fatalf("restore after the interrupted staging: %v", err)
	}
	if left := stagingEntries(t, dir); len(left) != 0 {
		t.Fatalf("the sweep did not put the entry back: %v", left)
	}
	if _, err := os.Stat(alphaArchive); err != nil {
		t.Fatalf("the archive entry did not come back at %s: %v", alphaArchive, err)
	}
	archived, err := fresh.ListArchived()
	if err != nil {
		t.Fatal(err)
	}
	var found *ArchivedMemory
	for i := range archived {
		if archived[i].ID == alpha.Memory.ID {
			found = &archived[i]
		}
	}
	if found == nil {
		t.Fatalf("the recovered archive entry is not listable: %+v", archived)
	}
	if found.Body != "alpha body" {
		t.Fatalf("the recovered archive entry lost its body: %+v", found)
	}
	if _, err := fresh.RestoreArchived(alphaArchive); err != nil {
		t.Fatalf("the recovered entry is not restorable: %v", err)
	}
	restored, ok, err := fresh.Read("alpha")
	if err != nil || !ok || restored.Body != "alpha body" {
		t.Fatalf("the recovered entry did not restore: ok=%v err=%v memory=%+v", ok, err, restored)
	}
}

func TestRestoreArchivedLeavesOneIndexLineAndNoStagingCopy(t *testing.T) {
	dir := testenv.TempDir(t)
	s := Store{Dir: dir}
	if _, err := s.Save(Memory{Name: "alpha", Description: "restored once", Type: TypeProject, Body: "alpha body"}); err != nil {
		t.Fatal(err)
	}
	archivePath, err := s.Archive("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RestoreArchived(archivePath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(archivePath); !os.IsNotExist(err) {
		t.Fatalf("the uninterrupted restore left the archive entry: %v", err)
	}
	if left := stagingEntries(t, dir); len(left) != 0 {
		t.Fatalf("the uninterrupted restore left staging copies: %v", left)
	}
	body := mustReadString(t, filepath.Join(dir, indexFile))
	if n := strings.Count(body, "(alpha.md)"); n != 1 {
		t.Fatalf("MEMORY.md names the restored fact %d times:\n%s", n, body)
	}
}

func TestRestoreArchivedStagingRenameFailurePublishesNothing(t *testing.T) {
	s, dir, archivePath := restoreFixture(t)
	withFault(t, func(op, path string) error {
		if op == "rename" && path == filepath.Join(".archive", filepath.Base(archivePath)) {
			return &os.PathError{Op: "rename", Path: path, Err: os.ErrPermission}
		}
		return nil
	})
	if _, err := s.RestoreArchived(archivePath); err == nil {
		t.Fatal("recovery reported success although the archive entry could not be staged")
	}
	memoryFault = nil
	if _, active := mustStore(t, s).Read("alpha"); active {
		t.Fatal("a restore that could not stage published the active file anyway")
	}
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("the archive entry left its place although staging failed: %v", err)
	}
	if left := stagingEntries(t, dir); len(left) != 0 {
		t.Fatalf("a failed staging left a hidden copy: %v", left)
	}
	if _, err := s.RestoreArchived(archivePath); err != nil {
		t.Fatalf("retry after a failed staging: %v", err)
	}
	if _, ok := mustStore(t, s).Read("alpha"); !ok {
		t.Fatal("the retry did not restore the fact")
	}
}

func TestRestoreArchivedSweepFailureDoesNotPublish(t *testing.T) {
	s, _, alphaArchive := restoreFixture(t)
	archived, err := s.ListArchived()
	if err != nil || len(archived) != 1 {
		t.Fatalf("fixture archive = %+v, err = %v", archived, err)
	}
	if _, err := s.Save(Memory{Name: "beta", Description: "second", Type: TypeProject, Body: "beta body"}); err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	betaArchive, err := s.Archive("beta")
	if err != nil {
		t.Fatal(err)
	}
	staging := stageArchivedEntryForTest(t, alphaArchive, archived[0].ID)

	withFault(t, func(op, path string) error {
		if op == "rename" && strings.HasPrefix(filepath.Base(path), restoreStagingPrefix) {
			return &os.PathError{Op: "rename", Path: path, Err: os.ErrPermission}
		}
		return nil
	})
	if _, err := s.RestoreArchived(betaArchive); err == nil {
		t.Fatal("recovery reported success although the sweep could not run")
	}
	memoryFault = nil
	if _, active := mustStore(t, s).Read("beta"); active {
		t.Fatal("a restore whose sweep failed published the active file anyway")
	}
	if _, err := os.Stat(betaArchive); err != nil {
		t.Fatalf("the sweep failure disturbed the entry being restored: %v", err)
	}
	if _, err := os.Stat(staging); err != nil {
		t.Fatalf("the sweep failure deleted the hidden copy: %v", err)
	}
	if _, err := s.RestoreArchived(betaArchive); err != nil {
		t.Fatalf("retry after a failed sweep: %v", err)
	}
	if _, ok := mustStore(t, s).Read("beta"); !ok {
		t.Fatal("the retry did not restore the fact")
	}
	if _, err := os.Stat(alphaArchive); err != nil {
		t.Fatalf("the retry did not put the recovered entry back: %v", err)
	}
}
