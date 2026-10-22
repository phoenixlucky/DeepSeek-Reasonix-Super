package boot

import (
	"slices"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/plugin"
)

// registerResourceTool adds the tool that reads an MCP server's declared
// resources, only when the user turned it on in their own config and a server
// in this session advertised the capability, so every other session's tool
// block stays as it was.
func registerResourceTool(host *plugin.Host, reg *tool.Registry, cfg *config.Config, plan mcpSpecPlan, live bool) {
	if !cfg.Tools.MCPResources {
		return
	}
	if len(cfg.Tools.Enabled) > 0 && !slices.Contains(cfg.Tools.Enabled, plugin.ResourceReadToolName) {
		return
	}
	if live {
		reg.Add(plugin.ResourceReadTool(host))
		return
	}
	for _, specs := range [][]plugin.Spec{plan.extra, plan.configured} {
		if slices.ContainsFunc(specs, host.DeclaresResources) {
			reg.Add(plugin.ResourceReadTool(host))
			return
		}
	}
}
