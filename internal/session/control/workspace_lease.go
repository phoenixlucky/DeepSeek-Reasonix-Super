// workspace_lease.go — what a controller says about the workspace write lease.
package control

import (
	"context"

	"reasonix/internal/state/workspacelease"
)

// WorkspaceLeaseState reports only whether this controller owns or is waiting
// for the Delivery workspace writer lease. It never exposes filesystem or
// process identity.
func (c *Controller) WorkspaceLeaseState() workspacelease.State {
	return c.workspaceLease.State()
}

// NameWorkspaceHolder gives this session a name for another session to see
// while waiting on the workspace this one is writing.
func (c *Controller) NameWorkspaceHolder(name func() string) {
	c.workspaceLease.SetHolder(name)
}

// yieldWorkspace returns two funcs around a wait on a person: yield gives the
// workspace write claim back, and restore, deferred, takes it again after any
// prompt lock deferred later has been released. A failure to take it back
// reaches err only when the wait itself succeeded. When another session was
// granted the claim meanwhile, the model is told and the receipt records it.
func (c *Controller) yieldWorkspace(ctx context.Context, err *error) (yield, restore func()) {
	resume := workspacelease.Resume(nil)
	yield = func() { resume = c.workspaceLease.Yield(ctx) }
	restore = func() {
		if resume == nil {
			return
		}
		contention, rerr := resume(ctx)
		if *err == nil {
			*err = rerr
		}
		if contention.Any() && c.executor != nil {
			c.executor.NoteClaimContended(contention)
		}
	}
	return yield, restore
}

// yieldWorkspaceNow is yieldWorkspace for a wait that starts at once.
func (c *Controller) yieldWorkspaceNow(ctx context.Context, err *error) func() {
	yield, restore := c.yieldWorkspace(ctx, err)
	yield()
	return restore
}

// yieldForRecoveryCard is how the Auto Guard gate waits on a person: the write
// claim is given back for the wait and retaken, or the failure reported, after.
func (c *Controller) yieldForRecoveryCard(ctx context.Context) func() error {
	var err error
	restore := c.yieldWorkspaceNow(ctx, &err)
	return func() error {
		restore()
		return err
	}
}
