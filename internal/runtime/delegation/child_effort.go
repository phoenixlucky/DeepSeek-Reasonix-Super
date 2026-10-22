package delegation

import (
	"context"
	"strings"

	"reasonix/internal/runtime/agent"
)

// childRuntimeRef is the model and effort a child is built for: the vision routing's
// answer, and for a child on the parent's own model with no effort of its own, the
// depth the parent session is on now. A parent that changed depth in place has a
// provider built with the old one, so inheriting means reading the session's.
func (t *TaskTool) childRuntimeRef(ctx context.Context, childModel, childEffort string) (string, string) {
	model := agent.VisionRefFor(ctx, childModel)
	effort := agent.VisionEffortFor(ctx, childModel, childEffort)
	if strings.TrimSpace(effort) == "" && model == strings.TrimSpace(t.baseModel) && model != "" {
		effort = agent.RequestEffortFromContext(ctx)
	}
	return model, effort
}

type childEffortKey struct{}

// withChildEffort marks the depth a child that shares the parent's provider
// instance starts on: the parent's, because that provider carries the built one.
// A child on a provider of its own is built for the effort it resolved to.
func (t *TaskTool) withChildEffort(runCtx, callCtx context.Context, modelRef, effortRef string) context.Context {
	if modelRef != "" || effortRef != "" {
		return runCtx
	}
	return context.WithValue(runCtx, childEffortKey{}, agent.RequestEffortFromContext(callCtx))
}
