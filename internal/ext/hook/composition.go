package hook

import (
	"fmt"
	"strings"

	"reasonix/internal/contract/agentcomposition"
)

func CompositionID(h ResolvedHook) string {
	s := h.Snapshot()
	s.Hook.Env = nil
	s.Hook.Timeout = 0
	s.Hook.Description = ""
	s.Approval = nil
	return "hook:" + agentcomposition.Digest(s)
}

func ResolveCompositionSelection(selection agentcomposition.Selection, hooks []ResolvedHook) agentcomposition.Selection {
	out := agentcomposition.Clone(selection)
	for i, m := range out.Members {
		if strings.HasPrefix(m.ID, "hook:") {
			continue
		}
		for index, h := range hooks {
			if m.ID == fmt.Sprintf("%s:%s:%d", h.Scope, h.Event, index) {
				out.Members[i].ID = CompositionID(h)
				break
			}
		}
	}
	return out
}

func CompositionHooks(hooks []ResolvedHook, d agentcomposition.Definition) []ResolvedHook {
	out := []ResolvedHook{}
	for _, h := range hooks {
		if h.IsPolicyGuard() || d.Hooks.Allows(CompositionID(h)) && (h.Scope != ScopePlugin || d.Extensions.Allows(h.Env["REASONIX_PLUGIN_NAME"])) {
			out = append(out, h)
		}
	}
	return out
}
