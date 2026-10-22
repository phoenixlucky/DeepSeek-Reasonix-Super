package control

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"reasonix/internal/platform/environment"
	"reasonix/internal/platform/gitcmd"
)

// repoRetryTimeout bounds the one second attempt to open a repository git did not answer about.
const repoRetryTimeout = 5 * time.Second

// repoSlot is the workspace's git identity. It is settled once; the single
// exception is a session whose open got no answer from git (a timeout, a failure
// to start), which gets one more attempt the first time the identity is read.
type repoSlot struct {
	mu      sync.Mutex
	repo    gitcmd.Repo
	retried bool
	open    func(ctx context.Context, dir string) (gitcmd.Repo, error) // nil is gitcmd.Open
}

// WorkspaceRepo is the git identity the session opened with. Host git reads the
// workspace through it rather than rediscovering the repository from files the
// session's own commands may have written, so it never changes once it names a
// repository, and git refusing a folder is never retried.
func (c *Controller) WorkspaceRepo() gitcmd.Repo {
	s := &c.repo
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.retried || s.repo.Absent != gitcmd.AbsenceUnavailable {
		return s.repo
	}
	s.retried = true
	// Only the folder the session opened, and only while its marker is the same
	// one: a marker that was removed or became another kind is not adopted.
	marker := environment.MarkerOf(s.repo.Dir)
	if marker.Path == "" || marker.Path != s.repo.MarkerPath || marker.Dir != s.repo.MarkerDir {
		return s.repo
	}
	open := s.open
	if open == nil {
		open = gitcmd.Open
	}
	ctx, cancel := context.WithTimeout(context.Background(), repoRetryTimeout)
	defer cancel()
	repo, err := open(ctx, s.repo.Dir)
	if err != nil {
		s.repo.Absent, s.repo.AbsentDetail = gitcmd.AbsenceOf(err)
		return s.repo
	}
	slog.Info("workspace git identity: opened on the second attempt", "dir", repo.Dir)
	s.repo = repo
	return s.repo
}
