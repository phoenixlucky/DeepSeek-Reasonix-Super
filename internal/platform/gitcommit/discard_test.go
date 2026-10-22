package gitcommit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
)

func discardRepo(t *testing.T) string {
	t.Helper()
	isolate(t)
	dir := testenv.TempDir(t)
	run(t, dir, "init", "-q")
	for name, body := range map[string]string{"keep.go": "package a\n", "edit.go": "package edit\n", "gone.go": "package gone\n", "mv.go": "package mv\nx\ny\nz\nw\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-qm", "base")
	return dir
}

func TestDiscardPutsAModifiedFileBackAsTheLastCommitHasItInIndexAndTree(t *testing.T) {
	dir := discardRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "edit.go"), []byte("package edit\nvar X = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "add", "edit.go")
	if err := os.WriteFile(filepath.Join(dir, "edit.go"), []byte("package edit\nvar X = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Discard(context.Background(), reopen(t, dir), "edit.go"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "edit.go"))
	if string(got) != "package edit\n" {
		t.Fatalf("edit.go = %q, want the committed content", got)
	}
	if out := run(t, dir, "status", "--porcelain"); out != "" {
		t.Fatalf("status after discard = %q, want clean (index and tree both restored)", out)
	}
}

func TestDiscardBringsADeletedFileBack(t *testing.T) {
	dir := discardRepo(t)
	if err := os.Remove(filepath.Join(dir, "gone.go")); err != nil {
		t.Fatal(err)
	}
	if err := Discard(context.Background(), reopen(t, dir), "gone.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gone.go")); err != nil {
		t.Fatalf("gone.go was not restored: %v", err)
	}
}

func TestDiscardTouchesOnlyTheNamedFile(t *testing.T) {
	dir := discardRepo(t)
	for _, name := range []string{"edit.go", "keep.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package changed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := Discard(context.Background(), reopen(t, dir), "edit.go"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "keep.go")); string(got) != "package changed\n" {
		t.Fatalf("keep.go = %q: discarding one file undid another", got)
	}
}

func TestDiscardRefusesWhatHasNothingToGoBackTo(t *testing.T) {
	dir := discardRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "fresh.go"), []byte("package fresh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "staged.go"), []byte("package staged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "add", "staged.go")
	run(t, dir, "mv", "mv.go", "moved.go")
	repo := reopen(t, dir)
	ctx := context.Background()
	for _, path := range []string{"fresh.go", "staged.go", "moved.go"} {
		if err := Discard(ctx, repo, path); !errors.Is(err, ErrNotDiscardable) {
			t.Errorf("Discard(%q) = %v, want ErrNotDiscardable", path, err)
		}
		if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
			t.Errorf("%s was removed by a refused discard", path)
		}
	}
}

func TestDiscardNamesOnlyWhatGitListsAsChanged(t *testing.T) {
	dir := discardRepo(t)
	repo := reopen(t, dir)
	ctx := context.Background()
	for _, path := range []string{"keep.go", "", "../x", "--all", "nope.go", ":(top)edit.go", "*"} {
		if err := Discard(ctx, repo, path); !errors.Is(err, ErrUnknownPath) {
			t.Errorf("Discard(%q) = %v, want ErrUnknownPath", path, err)
		}
	}
}
