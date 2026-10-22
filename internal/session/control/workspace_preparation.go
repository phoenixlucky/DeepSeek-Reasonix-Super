package control

import (
	"context"
	"errors"
	"sync"
	"time"

	"reasonix/internal/platform/worktree"
)

// WorkspacePreparationStatus is the host's answer for one worktree attempt.
type WorkspacePreparationStatus struct {
	ID            string `json:"id"`
	Phase         string `json:"phase"`
	SourceRoot    string `json:"sourceRoot"`
	Ref           string `json:"ref"`
	StartedAt     int64  `json:"startedAt"`
	Branch        string `json:"branch,omitempty"`
	WorkspaceRoot string `json:"workspaceRoot,omitempty"`
	Code          string `json:"code,omitempty"`
	Detail        string `json:"detail,omitempty"`
}

// WorkspacePreparation serializes cancellation with runtime publication.
// A ready operation has transferred ownership and cannot be rolled back.
type WorkspacePreparation struct {
	mu     sync.Mutex
	state  WorkspacePreparationStatus
	ctx    context.Context
	cancel context.CancelFunc
}

func NewWorkspacePreparation(parent context.Context, id, root, ref string) *WorkspacePreparation {
	ctx, cancel := context.WithCancel(parent)
	return &WorkspacePreparation{ctx: ctx, cancel: cancel, state: WorkspacePreparationStatus{
		ID: id, Phase: "validating", SourceRoot: root, Ref: ref, StartedAt: time.Now().UnixMilli(),
	}}
}

func (p *WorkspacePreparation) Context() context.Context { return p.ctx }

func (p *WorkspacePreparation) Status() WorkspacePreparationStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func (p *WorkspacePreparation) Update(progress worktree.Progress) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if preparationTerminal(p.state.Phase) {
		return
	}
	if p.ctx.Err() == nil {
		p.state.Phase = progress.Phase
	}
	if progress.Branch != "" {
		p.state.Branch = progress.Branch
	}
	if progress.WorkspaceRoot != "" {
		p.state.WorkspaceRoot = progress.WorkspaceRoot
	}
}

func (p *WorkspacePreparation) Cancel() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !preparationTerminal(p.state.Phase) {
		p.state.Phase = "cancelling"
		p.cancel()
	}
}

// Publish is the commit boundary. Cancellation that loses this lock observes
// ready; cancellation that wins prevents the host from replacing its runtime.
func (p *WorkspacePreparation) Publish(publish func() error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.ctx.Err(); err != nil {
		return err
	}
	if err := publish(); err != nil {
		return err
	}
	p.state.Phase = "ready"
	return nil
}

func (p *WorkspacePreparation) Finish(err error, cleanupFailed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if preparationTerminal(p.state.Phase) {
		return
	}
	p.state.Phase, p.state.Code = "failed", "workspace.worktree_failed"
	if cleanupFailed {
		p.state.Phase, p.state.Code = "cleanup_failed", "workspace.worktree_cleanup_failed"
	} else if errors.Is(p.ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		p.state.Phase, p.state.Code = "cancelled", "workspace.worktree_cancelled"
	} else if errors.Is(p.ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		p.state.Code = "workspace.worktree_timeout"
	} else if errors.Is(err, worktree.ErrSourceUnknown) {
		p.state.Code = "workspace.worktree_source_missing"
	}
	if err != nil {
		p.state.Detail = err.Error()
	}
}

func preparationTerminal(phase string) bool {
	switch phase {
	case "ready", "failed", "cancelled", "cleanup_failed":
		return true
	}
	return false
}
