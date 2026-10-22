package recovery

import (
	"context"
	"errors"
	"testing"
)

func waitingGate(t *testing.T, waiting func(context.Context) func() error) (*Gate, *[]string) {
	t.Helper()
	var order []string
	g := NewGate(Options{Mode: func() string { return "auto" }, Reviewer: staticReviewer{ReviewVerdict{
		Outcome: ReviewConfirm, ChangeKind: ChangeStrategy, Rationale: "public API direction belongs to the user",
	}}})
	g.opts.EmitPrompt = func(_ context.Context, taskID string, _ PendingProposal, _ *FailureEvent) (string, error) {
		order = append(order, "card")
		g.BindApprovalID(taskID, "w1")
		go func() { _ = g.Resolve("w1", ActionContinue, "") }()
		return "w1", nil
	}
	g.opts.WaitingOnPerson = func(ctx context.Context) func() error {
		order = append(order, "wait")
		inner := waiting(ctx)
		return func() error {
			order = append(order, "resumed")
			return inner()
		}
	}
	return g, &order
}

var planChange = Proposal{
	Tool: "todo_write", ReadOnly: true, PlanTransition: true,
	PlanBefore: "1. Keep current API [in_progress]",
	PlanAfter:  "1. Replace the public API [in_progress]",
}

func TestGateTellsTheHostItIsWaitingOnAPersonAndWhenItStops(t *testing.T) {
	g, order := waitingGate(t, func(context.Context) func() error { return func() error { return nil } })
	dec, err := g.BeforeMutation(context.Background(), planChange)
	if err != nil || !dec.Allow {
		t.Fatalf("decision = %+v, %v", dec, err)
	}
	if len(*order) != 3 || (*order)[0] != "card" || (*order)[1] != "wait" || (*order)[2] != "resumed" {
		t.Fatalf("order = %v, want card, wait, resumed", *order)
	}
}

func TestGateBlocksWhenWhatItSetAsideCannotBeTakenBack(t *testing.T) {
	g, _ := waitingGate(t, func(context.Context) func() error {
		return func() error { return errors.New("workspace.claim_timeout") }
	})
	dec, err := g.BeforeMutation(context.Background(), planChange)
	if err == nil || dec.Allow || !dec.Blocked {
		t.Fatalf("decision = %+v, %v; an approved call that cannot be put back under its claim must not run", dec, err)
	}
}
