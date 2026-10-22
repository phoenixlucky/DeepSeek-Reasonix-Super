package control

import (
	"context"

	"reasonix/internal/platform/gitcommit"
)

// DiscardRequest names one changed file whose change is to be thrown away.
type DiscardRequest struct {
	// Repo names the repository below the workspace; empty is the workspace's own.
	Repo string `json:"repo,omitempty"`
	Path string `json:"path"`
}

// DiscardChange puts one modified or deleted file back as the last commit has it.
// The change is gone afterwards and nothing keeps a copy, so it is refused while a
// turn runs, which may be writing the file, as a branch switch is.
func (c *Controller) DiscardChange(ctx context.Context, req DiscardRequest) error {
	repo, err := c.RepoFor(ctx, req.Repo)
	if err != nil {
		return err
	}
	c.commitMu.Lock()
	defer c.commitMu.Unlock()
	c.mu.Lock()
	busy := c.gate.busy()
	c.mu.Unlock()
	if busy {
		return ErrTurnRunning
	}
	return gitcommit.Discard(ctx, repo, req.Path)
}
