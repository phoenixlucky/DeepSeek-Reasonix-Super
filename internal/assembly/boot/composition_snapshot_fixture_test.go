package boot

import (
	"context"

	"reasonix/internal/contract/agentcomposition"
)

// Snapshot fixtures exercise the retained v1 writer and real recovery pipeline.
func buildSnapshotRuntime(ctx context.Context, opts Options) (*BuildResult, error) {
	b := &builder{timer: newPhaseTimer(), compositionBinding: func(v agentcomposition.Version, root string) *agentcomposition.Binding {
		return &agentcomposition.Binding{Version: v, Resolved: agentcomposition.Clone(v.Definition), Workspace: root, Resources: []agentcomposition.Resource{}}
	}}
	return b.build(ctx, opts)
}
