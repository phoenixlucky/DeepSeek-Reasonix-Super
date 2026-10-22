package gitstatus

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
)

func TestSummaryNamesRepoBranchAndCountsDirt(t *testing.T) {
	dir := testenv.TempDir(t)
	git(t, dir, "init", "-q", "-b", "main")
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt", "one\ntwo\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")

	s, ok := Summary(context.Background(), opened(t, dir))
	if !ok || s.Name != filepath.Base(dir) || s.Branch != "main" || s.Detached {
		t.Fatalf("clean tree: %+v ok=%v", s, ok)
	}
	if s.Added != 0 || s.Removed != 0 || s.Untracked != 0 {
		t.Fatalf("a clean tree has no dirt: %+v", s)
	}

	write("a.txt", "one\nthree\nfour\n")
	write("new.txt", "x\n")
	write("other.txt", "y\n")
	s, _ = Summary(context.Background(), opened(t, dir))
	if s.Added != 2 || s.Removed != 1 || s.Untracked != 2 {
		t.Fatalf("dirty tree: %+v, want +2 -1 ?2", s)
	}

	git(t, dir, "checkout", "-q", "--detach")
	s, _ = Summary(context.Background(), opened(t, dir))
	if !s.Detached || s.Branch == "" || s.Branch == "main" {
		t.Fatalf("detached HEAD names the commit: %+v", s)
	}
}

// The summary adds every numstat record. A rename is one record whose
// counts were not attached, so its lines were missing from the total while
// an ordinary edit beside it still counted.
func TestSummaryIncludesLinesFromARename(t *testing.T) {
	dir := testenv.TempDir(t)
	git(t, dir, "init", "-q", "-b", "main")
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("old.go", "package a\n\nfunc A() {}\n")
	write("keep.go", "package k\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")

	git(t, dir, "mv", "old.go", "new.go")
	write("new.go", "package a\n\nfunc A() {}\nfunc B() {}\n")
	git(t, dir, "add", "new.go")
	write("keep.go", "package k\n\nfunc K() {}\n")

	s, ok := Summary(context.Background(), opened(t, dir))
	if !ok {
		t.Fatal("summary of a repository was not ok")
	}
	if s.Added != 3 || s.Removed != 0 {
		t.Fatalf("summary = %+v, want +3 -0 (2 from the edit, 1 from the rename)", s)
	}
}

func TestSummaryOfANonRepositoryIsNotOK(t *testing.T) {
	dir := testenv.TempDir(t)
	if _, ok := Summary(context.Background(), opened(t, dir)); ok {
		t.Fatal("a directory outside any work tree has no summary")
	}
}

func TestSummaryCanceledContextDoesNotPublishPartialCounts(t *testing.T) {
	dir := repoWithCommit(t)
	repo := opened(t, dir)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if info, ok := Summary(ctx, repo); ok || info != (Info{}) {
		t.Fatalf("canceled summary = %+v ok=%v", info, ok)
	}
}
