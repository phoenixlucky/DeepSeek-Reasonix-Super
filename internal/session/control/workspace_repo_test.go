package control

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/platform/environment"
	"reasonix/internal/platform/gitcmd"
)

// markedFolder is a folder with a .git directory and the empty identity a session
// that got no answer from git would have opened with.
func markedFolder(t *testing.T, absent gitcmd.Absence) (string, gitcmd.Repo) {
	t.Helper()
	dir := testenv.TempDir(t)
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := environment.MarkerOf(dir)
	return dir, gitcmd.Repo{Dir: dir, Absent: absent, MarkerPath: marker.Path, MarkerDir: marker.Dir}
}

func slotWith(repo gitcmd.Repo, open func(context.Context, string) (gitcmd.Repo, error)) *Controller {
	c := &Controller{}
	c.repo = repoSlot{repo: repo, open: open}
	return c
}

func TestAnUnansweredOpenGetsOneSecondAttemptAndThenIsSettled(t *testing.T) {
	dir, empty := markedFolder(t, gitcmd.AbsenceUnavailable)
	found := gitcmd.Repo{Dir: dir, GitDir: filepath.Join(dir, ".git"), CommonDir: filepath.Join(dir, ".git"), WorkTree: dir}
	calls := 0
	c := slotWith(empty, func(_ context.Context, d string) (gitcmd.Repo, error) {
		calls++
		if d != dir {
			t.Errorf("retried %q, want the folder the session opened (%q)", d, dir)
		}
		return found, nil
	})
	if got := c.WorkspaceRepo(); !got.Valid() || got.WorkTree != dir {
		t.Fatalf("identity = %+v, want the repository the second open found", got)
	}
	// A third look, even if the files changed, is the same identity and no new open.
	os.RemoveAll(filepath.Join(dir, ".git"))
	if got := c.WorkspaceRepo(); got.WorkTree != dir || calls != 1 {
		t.Fatalf("identity moved or reopened: %+v after %d opens, want it settled after one", got, calls)
	}
}

func TestASecondAttemptThatFailsIsNotRepeated(t *testing.T) {
	_, empty := markedFolder(t, gitcmd.AbsenceUnavailable)
	calls := 0
	c := slotWith(empty, func(context.Context, string) (gitcmd.Repo, error) {
		calls++
		return gitcmd.Repo{}, context.DeadlineExceeded
	})
	for range 3 {
		if got := c.WorkspaceRepo(); got.Valid() || got.Absent != gitcmd.AbsenceUnavailable {
			t.Fatalf("identity = %+v, want still empty and unavailable", got)
		}
	}
	if calls != 1 {
		t.Fatalf("opened %d times, want exactly one retry", calls)
	}
}

func TestGitRefusingAFolderIsNeverRetried(t *testing.T) {
	_, empty := markedFolder(t, gitcmd.AbsenceRefused)
	calls := 0
	c := slotWith(empty, func(context.Context, string) (gitcmd.Repo, error) {
		calls++
		return gitcmd.Repo{}, errors.New("unused")
	})
	c.WorkspaceRepo()
	c.WorkspaceRepo()
	if calls != 0 {
		t.Fatalf("a refusal was retried %d times", calls)
	}
}

func TestASessionThatOpenedItsRepositoryNeverOpensAgain(t *testing.T) {
	dir := testenv.TempDir(t)
	calls := 0
	c := slotWith(gitcmd.Repo{Dir: dir, GitDir: dir, CommonDir: dir, WorkTree: dir}, func(context.Context, string) (gitcmd.Repo, error) { calls++; return gitcmd.Repo{}, nil })
	c.WorkspaceRepo()
	if calls != 0 {
		t.Fatalf("a session with an identity opened again %d times", calls)
	}
}

func TestAMarkerThatWentAwayOrChangedKindIsNotAdopted(t *testing.T) {
	for name, change := range map[string]func(dir string){
		"removed": func(dir string) { os.RemoveAll(filepath.Join(dir, ".git")) },
		"became a file": func(dir string) {
			os.RemoveAll(filepath.Join(dir, ".git"))
			os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: elsewhere\n"), 0o644)
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir, empty := markedFolder(t, gitcmd.AbsenceUnavailable)
			change(dir)
			calls := 0
			c := slotWith(empty, func(context.Context, string) (gitcmd.Repo, error) {
				calls++
				return gitcmd.Repo{Dir: dir, GitDir: dir, CommonDir: dir, WorkTree: dir}, nil
			})
			if got := c.WorkspaceRepo(); got.Valid() || calls != 0 {
				t.Fatalf("adopted %+v after %d opens: a marker that is gone or another kind is not the folder the session opened", got, calls)
			}
		})
	}
}
