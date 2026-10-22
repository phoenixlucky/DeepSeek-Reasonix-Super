package gitrepos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
)

func initRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v\n%s", dir, err, out)
	}
}

func dirsOf(r Result) []string {
	out := make([]string, len(r.Repos))
	for i, f := range r.Repos {
		out[i] = f.Dir
	}
	return out
}

func TestAWorkspaceThatIsARepositoryIsJustThatOne(t *testing.T) {
	root := testenv.TempDir(t)
	initRepo(t, root)
	initRepo(t, filepath.Join(root, "inner"))
	got, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Repos) != 1 || got.Repos[0].Dir != "" {
		t.Fatalf("repos = %v, want the workspace alone", dirsOf(got))
	}
}

func TestAFolderOfProjectsListsEachRepositoryBelowIt(t *testing.T) {
	root := testenv.TempDir(t)
	initRepo(t, filepath.Join(root, "api"))
	initRepo(t, filepath.Join(root, "web"))
	initRepo(t, filepath.Join(root, "group", "tools"))
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	initRepo(t, filepath.Join(root, "a", "b", "too-deep"))
	got, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"api", "group/tools", "web"}
	if fmt.Sprint(dirsOf(got)) != fmt.Sprint(want) {
		t.Fatalf("repos = %v, want %v (a plain folder is not one, a great-grandchild is out of depth)", dirsOf(got), want)
	}
	for _, f := range got.Repos {
		if f.Repo.WorkTree == "" || !f.Repo.Valid() {
			t.Fatalf("%s has no resolved identity: %+v", f.Dir, f.Repo)
		}
	}
}

func TestARepositoryIsNotEnteredLookingForMore(t *testing.T) {
	root := testenv.TempDir(t)
	initRepo(t, filepath.Join(root, "outer"))
	initRepo(t, filepath.Join(root, "outer", "vendor", "inner"))
	got, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(dirsOf(got)) != "[outer]" {
		t.Fatalf("repos = %v, want only outer", dirsOf(got))
	}
}

func TestASymbolicLinkIsNotFollowedOutOfTheWorkspace(t *testing.T) {
	root, elsewhere := testenv.TempDir(t), testenv.TempDir(t)
	initRepo(t, filepath.Join(elsewhere, "secret"))
	if err := os.Symlink(filepath.Join(elsewhere, "secret"), filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	got, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Repos) != 0 {
		t.Fatalf("a linked repository outside the workspace was listed: %v", dirsOf(got))
	}
}

func TestAFolderInsideSomeOtherRepositoryIsNotARepositoryOfItsOwn(t *testing.T) {
	root := testenv.TempDir(t)
	initRepo(t, root)
	sub := filepath.Join(root, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := topOfItsOwnTree(context.Background(), sub); ok {
		t.Fatal("a plain folder inside a repository was taken for a repository")
	}
}

func TestTheNumberOfRepositoriesKeptIsBounded(t *testing.T) {
	root := testenv.TempDir(t)
	for i := range MaxRepos + 3 {
		initRepo(t, filepath.Join(root, fmt.Sprintf("p%02d", i)))
	}
	got, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Repos) != MaxRepos || !got.Truncated {
		t.Fatalf("kept %d truncated=%v, want %d and truncated", len(got.Repos), got.Truncated, MaxRepos)
	}
}

func TestFindAnswersOnlyWhatTheSearchFound(t *testing.T) {
	root, elsewhere := testenv.TempDir(t), testenv.TempDir(t)
	initRepo(t, filepath.Join(root, "api"))
	initRepo(t, filepath.Join(root, "group", "tools"))
	initRepo(t, filepath.Join(root, "a", "b", "deep"))
	initRepo(t, filepath.Join(elsewhere, "outside"))
	if err := os.Symlink(filepath.Join(elsewhere, "outside"), filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	found, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	for _, ok := range []string{"api", "group/tools"} {
		if repo, err := found.Find(ok); err != nil || !repo.Valid() {
			t.Errorf("Find(%q) = %v, %v; want its repository", ok, repo.WorkTree, err)
		}
	}
	for _, bad := range []string{"", "..", "../" + filepath.Base(elsewhere) + "/outside", "/etc", "a/b/deep", "link", "api/..", "api//x", "group", "missing", "./api", "api/sub/x", `api\x`, `..\api`} {
		if _, err := found.Find(bad); !errors.Is(err, ErrNotListed) {
			t.Errorf("Find(%q) err = %v, want ErrNotListed", bad, err)
		}
	}
}

func TestFindNamesNoChildWhenTheWorkspaceIsTheRepository(t *testing.T) {
	root := testenv.TempDir(t)
	initRepo(t, root)
	initRepo(t, filepath.Join(root, "inner"))
	found, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := found.Find(""); err != nil {
		t.Fatalf("the workspace's own repository: %v", err)
	}
	if _, err := found.Find("inner"); !errors.Is(err, ErrNotListed) {
		t.Fatalf("a child of a repository workspace = %v, want ErrNotListed", err)
	}
}

func TestTheSearchIsKeptUntilTheWorkspaceStructureChanges(t *testing.T) {
	root := testenv.TempDir(t)
	initRepo(t, filepath.Join(root, "api"))
	var f Finder
	ctx := context.Background()
	first, err := f.Find(ctx, root)
	if err != nil || len(first.Repos) != 1 {
		t.Fatalf("first = %v, %v", dirsOf(first), err)
	}
	if _, err := f.Find(ctx, root); err != nil || f.runs != 1 {
		t.Fatalf("an unchanged workspace searched again: runs = %d (%v)", f.runs, err)
	}
	// Work inside a repository is not a change of the structure.
	if err := os.WriteFile(filepath.Join(root, "api", "main.go"), []byte("package a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Find(ctx, root); err != nil || f.runs != 1 {
		t.Fatalf("a file written in a repository made the search run again: runs = %d", f.runs)
	}
	time.Sleep(20 * time.Millisecond)
	initRepo(t, filepath.Join(root, "web"))
	again, err := f.Find(ctx, root)
	if err != nil || f.runs != 2 || fmt.Sprint(dirsOf(again)) != "[api web]" {
		t.Fatalf("a new repository was not found: runs = %d, repos = %v, %v", f.runs, dirsOf(again), err)
	}
	time.Sleep(20 * time.Millisecond)
	initRepo(t, filepath.Join(root, "group", "tools"))
	deep, err := f.Find(ctx, root)
	if err != nil || f.runs != 3 || fmt.Sprint(dirsOf(deep)) != "[api group/tools web]" {
		t.Fatalf("a new repository two levels down was not found: runs = %d, repos = %v, %v", f.runs, dirsOf(deep), err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := os.RemoveAll(filepath.Join(root, "web")); err != nil {
		t.Fatal(err)
	}
	gone, err := f.Find(ctx, root)
	if err != nil || f.runs != 4 || fmt.Sprint(dirsOf(gone)) != "[api group/tools]" {
		t.Fatalf("a removed repository is still listed: runs = %d, repos = %v, %v", f.runs, dirsOf(gone), err)
	}
}

func TestACachedSearchIsOnlyForTheWorkspaceItWasMadeFor(t *testing.T) {
	a, b := testenv.TempDir(t), testenv.TempDir(t)
	initRepo(t, filepath.Join(a, "one"))
	initRepo(t, filepath.Join(b, "two"))
	var f Finder
	ra, _ := f.Find(context.Background(), a)
	rb, _ := f.Find(context.Background(), b)
	if fmt.Sprint(dirsOf(ra)) != "[one]" || fmt.Sprint(dirsOf(rb)) != "[two]" || f.runs != 2 {
		t.Fatalf("a = %v, b = %v, runs = %d", dirsOf(ra), dirsOf(rb), f.runs)
	}
}
