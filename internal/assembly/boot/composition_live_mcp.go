package boot

import (
	"context"

	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/plugin"
)

func registerLiveCompositionMCP(ctx context.Context, host *plugin.Host, reg *tool.Registry, plan mcpSpecPlan) ([]plugin.Spec, map[string]bool) {
	known := map[string]bool{}
	for _, specs := range [][]plugin.Spec{plan.configured, plan.extra} {
		for _, spec := range specs {
			plugin.ApplyDisabledMCPPolicy(reg, spec)
			cache, _ := plugin.LoadCachedSchemaForSpec(spec)
			known[spec.Name] = cache != nil && len(cache.Tools) > 0
			addTools(reg, plugin.LazyToolset(spec, cache, host, reg, ctx, false))
			if (plan.alwaysLoad[spec.Name] || plan.autoConnect[spec.Name]) && plugin.ResolveStoredAuthorization(ctx, spec).ServerAuthorized() {
				host.EnsureConnectedInBackground(ctx, spec)
			}
		}
	}
	return plan.configured, known
}
