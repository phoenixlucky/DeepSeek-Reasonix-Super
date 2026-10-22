package control

import (
	"context"
	"errors"

	"reasonix/internal/contract/config"
	"reasonix/internal/platform/gitpush"
)

// ErrWorkspaceNotTrusted refuses a push from a workspace the person has not
// trusted: a push is the one git action that leaves the machine.
var ErrWorkspaceNotTrusted = errors.New("control: the workspace is not trusted, so nothing is pushed from it")

// PushRequest is a push the person confirmed: the repository, and the plan the
// preview showed them. A repository that no longer matches the plan is refused.
type PushRequest struct {
	// Repo names the repository below the workspace; empty is the workspace's own.
	Repo string       `json:"repo,omitempty"`
	Plan gitpush.Plan `json:"plan"`
}

// GitPushPreview says what pushing the repository's current branch would send
// and where, without reaching the remote.
func (c *Controller) GitPushPreview(ctx context.Context, repoDir string) (gitpush.Plan, error) {
	if c.WorkspaceTrust() != config.WorkspaceTrusted {
		return gitpush.Plan{}, ErrWorkspaceNotTrusted
	}
	repo, err := c.RepoFor(ctx, repoDir)
	if err != nil {
		return gitpush.Plan{}, err
	}
	return gitpush.Preview(ctx, repo)
}

// GitPush sends the confirmed branch to its upstream. It shares the commit lock,
// so a commit is never half-made while it goes.
func (c *Controller) GitPush(ctx context.Context, req PushRequest) (gitpush.Result, error) {
	if c.WorkspaceTrust() != config.WorkspaceTrusted {
		return gitpush.Result{}, ErrWorkspaceNotTrusted
	}
	repo, err := c.RepoFor(ctx, req.Repo)
	if err != nil {
		return gitpush.Result{}, err
	}
	c.commitMu.Lock()
	defer c.commitMu.Unlock()
	return gitpush.Push(ctx, repo, req.Plan)
}
