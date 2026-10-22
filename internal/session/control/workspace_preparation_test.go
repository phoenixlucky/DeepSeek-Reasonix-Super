package control

import (
	"context"
	"errors"
	"testing"
	"time"

	"reasonix/internal/platform/worktree"
)

func TestWorkspacePreparationCancelWinsBeforePublication(t *testing.T) {
	op := NewWorkspacePreparation(t.Context(), "test", "/source", "refs/heads/main")
	op.Cancel()
	called := false
	if err := op.Publish(func() error { called = true; return nil }); !errors.Is(err, context.Canceled) || called {
		t.Fatalf("publish after cancellation: %v called=%v", err, called)
	}
	op.Finish(context.Canceled, false)
	if got := op.Status(); got.Phase != "cancelled" {
		t.Fatalf("status=%+v", got)
	}
}

func TestWorkspacePreparationPublicationWinsBeforeCancel(t *testing.T) {
	op := NewWorkspacePreparation(t.Context(), "test", "/source", "refs/heads/main")
	if err := op.Publish(func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	op.Cancel()
	op.Finish(nil, false)
	if got := op.Status(); got.Phase != "ready" || op.Context().Err() != nil {
		t.Fatalf("committed operation was cancelled: %+v", got)
	}
}

func TestWorkspacePreparationCleanupFailureCannotBecomeCancelled(t *testing.T) {
	op := NewWorkspacePreparation(t.Context(), "test", "/source", "refs/heads/main")
	op.Cancel()
	op.Finish(errors.New("cleanup unavailable"), true)
	if got := op.Status(); got.Phase != "cleanup_failed" || got.Code != "workspace.worktree_cleanup_failed" {
		t.Fatalf("uncertain cleanup must stay visible: %+v", got)
	}
}

func TestWorkspacePreparationPublicationExcludesConcurrentCancel(t *testing.T) {
	op := NewWorkspacePreparation(t.Context(), "test", "/source", "HEAD")
	entered, release, published, cancelled := make(chan struct{}), make(chan struct{}), make(chan error, 1), make(chan struct{})
	go func() {
		published <- op.Publish(func() error { close(entered); <-release; return nil })
	}()
	<-entered
	go func() { op.Cancel(); close(cancelled) }()
	close(release)
	if err := <-published; err != nil {
		t.Fatal(err)
	}
	<-cancelled
	if got := op.Status(); got.Phase != "ready" || op.Context().Err() != nil {
		t.Fatalf("publication lost ownership: %+v", got)
	}
}

func TestWorkspacePreparationTerminalIdentityCannotChange(t *testing.T) {
	op := NewWorkspacePreparation(t.Context(), "test", "/source", "HEAD")
	op.Update(worktree.Progress{Phase: "binding", Branch: "task", WorkspaceRoot: "/target"})
	if err := op.Publish(func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	before := op.Status()
	op.Update(worktree.Progress{Phase: "checking_out", Branch: "other", WorkspaceRoot: "/other"})
	if got := op.Status(); got != before {
		t.Fatalf("late progress changed a published identity: %+v", got)
	}
}

func TestWorkspacePreparationDeadlineHasTypedRetryableOutcome(t *testing.T) {
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	op := NewWorkspacePreparation(ctx, "test", "/source", "HEAD")
	op.Finish(ctx.Err(), false)
	if got := op.Status(); got.Phase != "failed" || got.Code != "workspace.worktree_timeout" {
		t.Fatalf("deadline identity was lost: %+v", got)
	}
}
