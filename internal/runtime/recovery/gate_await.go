package recovery

import "context"

// awaitCard blocks until the card is resolved or ctx ends.
func (g *Gate) awaitCard(ctx context.Context, taskID, approvalID string, reply chan resolvePayload, proposal Proposal, gen uint64) (Decision, error) {
	select {
	case payload := <-reply:
		decision, err := g.decisionFromResolve(payload)
		if err == nil && decision.Allow && proposal.PlanTransition {
			decision.AuthorizePlanReplacement = true
		}
		decision.Generation = gen
		return decision, err
	case <-ctx.Done():
		g.mu.Lock()
		delete(g.waiters, approvalID)
		delete(g.taskOf, approvalID)
		delete(g.pending, approvalID)
		delete(g.awaiting, taskID)
		g.mu.Unlock()
		g.persist()
		return Decision{Allow: false, Blocked: true, Message: "blocked: Auto Guard confirmation cancelled", Generation: gen}, ctx.Err()
	}
}
