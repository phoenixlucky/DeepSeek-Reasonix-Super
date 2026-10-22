package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/base/workspaceid"
)

func TestProjectRootUsesGitIdentityAndPreservesSelectedSubdirectory(t *testing.T) {
	requireGit(t)
	repo, managed := initRepo(t), testenv.TempDir(t)
	result, err := Create(t.Context(), opened(t, repo), managed)
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", filepath.Join("packages", "app")} {
		root := filepath.Join(result.WorkspaceRoot, suffix)
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if got, want := ProjectRoot(root, managed), filepath.Join(result.SourceRoot, suffix); got != want {
			t.Errorf("%s: project %s, want %s", root, got, want)
		}
	}
	if got := ProjectRoot(repo, managed); got != repo {
		t.Errorf("original directory moved: %s", got)
	}
	if got := ProjectRoot(result.WorkspaceRoot, testenv.TempDir(t)); got != result.WorkspaceRoot {
		t.Errorf("a separately managed checkout was folded: %s", got)
	}
	unknown := filepath.Join(managed, "same-name", filepath.Base(repo))
	if err := os.MkdirAll(unknown, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ProjectRoot(unknown, managed); got != unknown {
		t.Errorf("a folder name was mistaken for a Git relationship: %s", got)
	}
	if err := os.Remove(filepath.Join(result.WorkspaceRoot, ".git")); err != nil {
		t.Fatal(err)
	}
	if got := ProjectRoot(result.WorkspaceRoot, managed); got != result.WorkspaceRoot {
		t.Errorf("an unreadable identity must retain its directory: %s", got)
	}
}

func TestProjectRootRejectsCopiedAndMissingWorktreeRegistration(t *testing.T) {
	requireGit(t)
	repo, managed := initRepo(t), testenv.TempDir(t)
	result, err := Create(t.Context(), opened(t, repo), managed)
	if err != nil {
		t.Fatal(err)
	}
	marker, err := os.ReadFile(filepath.Join(result.WorktreeRoot, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	copy := filepath.Join(managed, "restored", "project")
	if err := os.MkdirAll(copy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(copy, ".git"), marker, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ProjectRoot(copy, managed); got != copy {
		t.Errorf("copied Git metadata borrowed another checkout's identity: %s", got)
	}
	info := workspaceid.Describe(result.WorktreeRoot)
	if err := os.Remove(filepath.Join(info.GitDir, "gitdir")); err != nil {
		t.Fatal(err)
	}
	if got := ProjectRoot(result.WorkspaceRoot, managed); got != result.WorkspaceRoot {
		t.Errorf("missing worktree registration must retain its own project: %s", got)
	}
}

func TestProjectRootDoesNotBorrowRepositoryOutsideManagedStorage(t *testing.T) {
	requireGit(t)
	repo, managed := initRepo(t), testenv.TempDir(t)
	result, err := Create(t.Context(), opened(t, repo), managed)
	if err != nil {
		t.Fatal(err)
	}
	nestedManaged := filepath.Join(result.WorktreeRoot, "other-worktrees")
	root := filepath.Join(nestedManaged, "unregistered")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ProjectRoot(root, nestedManaged); got != root {
		t.Fatalf("a Git marker outside managed storage claimed the folder: %s", got)
	}
}

func TestProjectRootConfinesRegistrationReadsToItsGitDirectory(t *testing.T) {
	requireGit(t)
	for _, outside := range []bool{false, true} {
		t.Run(map[bool]string{false: "inside symlink", true: "escaping symlink"}[outside], func(t *testing.T) {
			repo, managed := initRepo(t), testenv.TempDir(t)
			result, err := Create(t.Context(), opened(t, repo), managed)
			if err != nil {
				t.Fatal(err)
			}
			info := workspaceid.Describe(result.WorktreeRoot)
			registration := filepath.Join(info.GitDir, "gitdir")
			body, err := os.ReadFile(registration)
			if err != nil {
				t.Fatal(err)
			}
			dir := info.GitDir
			if outside {
				dir = testenv.TempDir(t)
			}
			backlink := filepath.Join(dir, "backlink")
			if err := os.WriteFile(backlink, body, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(registration); err != nil {
				t.Fatal(err)
			}
			link := backlink
			if !outside {
				link = filepath.Base(backlink)
			}
			if err := os.Symlink(link, registration); err != nil {
				t.Skipf("file symlinks unavailable: %v", err)
			}
			want := result.SourceRoot
			if outside {
				want = result.WorkspaceRoot
			}
			if got := ProjectRoot(result.WorkspaceRoot, managed); got != want {
				t.Fatalf("project root = %q, want %q", got, want)
			}
		})
	}
}

func TestProjectRootAcceptsRelativeRegistrationBacklink(t *testing.T) {
	requireGit(t)
	repo, managed := initRepo(t), testenv.TempDir(t)
	result, err := Create(t.Context(), opened(t, repo), managed)
	if err != nil {
		t.Fatal(err)
	}
	info := workspaceid.Describe(result.WorktreeRoot)
	backlink, err := filepath.Rel(info.GitDir, filepath.Join(info.CheckoutRoot, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(info.GitDir, "gitdir"), []byte(backlink+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ProjectRoot(result.WorkspaceRoot, managed); got != result.SourceRoot {
		t.Fatalf("relative backlink lost project membership: %q, want %q", got, result.SourceRoot)
	}
}
