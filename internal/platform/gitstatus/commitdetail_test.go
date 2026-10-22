package gitstatus

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func head(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestACommitListsWhatItChangedAgainstItsParent(t *testing.T) {
	dir := testenv.TempDir(t)
	git(t, dir, "init", "-q")
	write(t, dir, "keep.go", "package a\n")
	write(t, dir, "old.go", "package old\nline1\nline2\nline3\nline4\n")
	write(t, dir, "gone.go", "package gone\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	root := head(t, dir)

	write(t, dir, "keep.go", "package a\nvar X = 1\n")
	git(t, dir, "mv", "old.go", "new.go")
	git(t, dir, "rm", "-q", "gone.go")
	write(t, dir, "added.bin", "\x00\x01\x02")
	write(t, dir, "sub/deep.go", "package sub\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "second")
	second := head(t, dir)

	ctx := context.Background()
	got, truncated, ok, err := CommitFiles(ctx, opened(t, dir), second)
	if err != nil || !ok || truncated {
		t.Fatalf("CommitFiles = %v ok=%v truncated=%v", err, ok, truncated)
	}
	by := map[string]CommitFile{}
	for _, f := range got {
		by[f.Path] = f
	}
	if f := by["keep.go"]; f.Status != "M" || f.Insertions == nil || *f.Insertions != 1 {
		t.Errorf("keep.go = %+v, want modified with one insertion", f)
	}
	if f := by["new.go"]; f.Status != "R" || f.OldPath != "old.go" {
		t.Errorf("new.go = %+v, want a rename from old.go", f)
	}
	if f := by["gone.go"]; f.Status != "D" {
		t.Errorf("gone.go = %+v, want deleted", f)
	}
	if f := by["added.bin"]; f.Status != "A" || f.Insertions != nil {
		t.Errorf("added.bin = %+v, want added with no counts (a binary file was not counted, which is not zero)", f)
	}
	if f := by["sub/deep.go"]; f.Status != "A" {
		t.Errorf("sub/deep.go = %+v", f)
	}

	first, _, _, err := CommitFiles(ctx, opened(t, dir), root)
	if err != nil || len(first) != 3 {
		t.Fatalf("a root commit lists its whole tree against the empty tree: %v %+v", err, first)
	}
	for _, f := range first {
		if f.Status != "A" {
			t.Errorf("%s in the root commit is %q, want A", f.Path, f.Status)
		}
	}

	text, trunc, err := CommitDiff(ctx, opened(t, dir), second, "keep.go")
	if err != nil || trunc || !strings.Contains(text, "+var X = 1") {
		t.Fatalf("CommitDiff = %q trunc=%v err=%v", text, trunc, err)
	}
	if strings.Contains(text, "package old") {
		t.Fatal("a diff of one path carried another path's lines")
	}
}

func TestACommitThatIsNotOneIsRefused(t *testing.T) {
	dir := testenv.TempDir(t)
	git(t, dir, "init", "-q")
	write(t, dir, "a.go", "package a\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	repo := opened(t, dir)
	ctx := context.Background()
	for _, id := range []string{"", "--output=/tmp/x", "HEAD", "main", "abc", "zzzzzzzz", strings.Repeat("a", 65), "-p", "$(touch x)"} {
		if _, _, _, err := CommitFiles(ctx, repo, id); !errors.Is(err, ErrBadCommit) {
			t.Errorf("CommitFiles(%q) err = %v, want ErrBadCommit", id, err)
		}
		if _, _, err := CommitDiff(ctx, repo, id, "a.go"); !errors.Is(err, ErrBadCommit) {
			t.Errorf("CommitDiff(%q) err = %v, want ErrBadCommit", id, err)
		}
	}
	if _, _, _, err := CommitFiles(ctx, repo, "0000000000000000000000000000000000000000"); !errors.Is(err, ErrNoSuchCommit) {
		t.Errorf("an unknown well-formed id err = %v, want ErrNoSuchCommit", err)
	}
	if _, _, err := CommitDiff(ctx, repo, head(t, dir), "../outside"); !errors.Is(err, ErrPathOutsideTree) {
		t.Errorf("a path out of the tree err = %v, want ErrPathOutsideTree", err)
	}
}

func TestAMergeCommitIsMeasuredAgainstItsFirstParent(t *testing.T) {
	dir := testenv.TempDir(t)
	git(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "a.go", "package a\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	git(t, dir, "checkout", "-q", "-b", "side")
	write(t, dir, "side.go", "package side\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "side")
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "main.go", "package main\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "main")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "merge", "side")
	got, _, _, err := CommitFiles(context.Background(), opened(t, dir), head(t, dir))
	if err != nil || len(got) != 1 || got[0].Path != "side.go" {
		t.Fatalf("a merge shows what it brought in from the other side: %v %+v", err, got)
	}
}

func TestACommitOfAFolderThatIsNotARepositoryAnswersNotOK(t *testing.T) {
	_, _, ok, err := CommitFiles(context.Background(), opened(t, testenv.TempDir(t)), "abcdef0")
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want a quiet not-a-repository", ok, err)
	}
}

func TestAGitThatCannotAnswerIsNotAMissingCommit(t *testing.T) {
	dir := testenv.TempDir(t)
	git(t, dir, "init", "-q")
	write(t, dir, "a.go", "package a\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	repo := opened(t, dir)
	id := head(t, dir)
	write(t, filepath.Join(dir, ".git"), "HEAD", "garbage that is not a ref\n")
	_, _, _, err := CommitFiles(context.Background(), repo, id)
	if err == nil || errors.Is(err, ErrNoSuchCommit) {
		t.Fatalf("err = %v, want git's own failure and not ErrNoSuchCommit", err)
	}
}
