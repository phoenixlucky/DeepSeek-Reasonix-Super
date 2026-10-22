package workspacelease

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
)

func siblingOwners(t *testing.T, opts ...Option) (a, b *Owner, repo string) {
	t.Helper()
	repo, locks := testenv.TempDir(t), testenv.TempDir(t)
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(repo, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a, err := New(filepath.Join(repo, "a"), locks, nil, opts...)
	if err != nil {
		t.Fatal(err)
	}
	b, err = New(filepath.Join(repo, "b"), locks, nil, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a.EndRun()
		b.EndRun()
	})
	a.BeginRun()
	b.BeginRun()
	return a, b, repo
}

func TestSiblingFoldersUnderOneRepositoryEachHoldOnlyTheirOwnFolder(t *testing.T) {
	a, b, _ := siblingOwners(t, WithFolderClaim())
	if err := a.AcquireWrite(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := within(t, b, nil, time.Second); err != nil {
		t.Fatalf("a sibling folder waited on a session that only writes its own folder: %v", err)
	}
}

func TestAFolderWriterStillConflictsWithAnotherSessionsPathInsideIt(t *testing.T) {
	a, b, repo := siblingOwners(t, WithFolderClaim())
	if err := a.AcquireWrite(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := within(t, b, []string{filepath.Join(repo, "a", "x.go")}, 200*time.Millisecond); err == nil {
		t.Fatal("a path inside a folder another session holds was granted")
	}
}

func TestADeclaredPathOutsideTheFolderIsClaimedAsDeclared(t *testing.T) {
	a, b, repo := siblingOwners(t, WithFolderClaim())
	if err := a.AcquirePaths(context.Background(), []string{filepath.Join(repo, "shared.go")}); err != nil {
		t.Fatal(err)
	}
	if err := within(t, b, []string{filepath.Join(repo, "shared.go")}, 200*time.Millisecond); err == nil {
		t.Fatal("a declared path in the repository, outside both folders, was granted twice")
	}
	if err := within(t, b, nil, time.Second); err != nil {
		t.Fatalf("an undeclared writer in b waited on a's single declared file: %v", err)
	}
}

func TestAnUnresolvablePathStillClaimsTheWholeRepository(t *testing.T) {
	a, b, _ := siblingOwners(t, WithFolderClaim())
	if err := a.AcquirePaths(context.Background(), []string{"  "}); err != nil {
		t.Fatal(err)
	}
	if err := within(t, b, nil, 200*time.Millisecond); err == nil {
		t.Fatal("a path that could not be resolved was narrowed to the session's folder")
	}
}

func TestWithoutAConfinedShellSiblingFoldersStillWaitOnEachOther(t *testing.T) {
	a, b, _ := siblingOwners(t)
	if err := a.AcquireWrite(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := within(t, b, nil, 200*time.Millisecond); err == nil {
		t.Fatal("an unconfined writer was narrowed to its folder; its write could land in the sibling's")
	}
}
