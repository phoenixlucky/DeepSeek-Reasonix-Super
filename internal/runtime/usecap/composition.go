package usecap

import (
	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/plugin"
)

func (r *MCPCapabilityRuntime) ConfigureComposition(d agentcomposition.Definition) {
	r.dispatchMu.Lock()
	defer r.dispatchMu.Unlock()
	policy := agentcomposition.Clone(d)
	r.composition = &policy
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, server := range r.servers {
		if !r.compositionAllows(server.spec) {
			delete(r.servers, name)
			r.state.clearServer(name)
		}
	}
}

func (r *MCPCapabilityRuntime) compositionAllows(spec plugin.Spec) bool {
	return r.composition == nil || r.composition.MCP.Allows(spec.Name) && (spec.Package == "" || r.composition.Extensions.Allows(spec.Package))
}

func (r *MCPCapabilityRuntime) BindRegisteredTool(target tool.Tool) tool.Tool {
	if bound, ok := target.(*runtimeBoundMCPTool); ok && bound.proxy.runtime == r {
		return target
	}
	meta, ok := target.(tool.MCPMetadata)
	if !ok {
		return target
	}
	return &runtimeBoundMCPTool{proxy: r.NewFrontend(nil, nil), target: target, server: meta.MCPServerName(), authorized: tool.IsMCPServerAuthorized(target)}
}

func mcpToolMatchesRuntime(target tool.Tool, spec plugin.Spec) bool {
	for {
		bound, ok := target.(*runtimeBoundMCPTool)
		if !ok {
			return plugin.MCPToolMatchesSpec(target, spec)
		}
		target = bound.target
	}
}

// FreezeComposition keeps settings edits from widening this session's catalog.
func (r *MCPCapabilityRuntime) FreezeComposition() {
	if r == nil {
		return
	}
	r.dispatchMu.Lock()
	defer r.dispatchMu.Unlock()
	r.compositionFrozen = true
}
