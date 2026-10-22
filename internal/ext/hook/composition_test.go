package hook

import (
	"testing"

	"reasonix/internal/contract/agentcomposition"
)

func TestCompositionHookSelectionSurvivesReorderWithoutWidening(t *testing.T) {
	first := ResolvedHook{HookConfig: HookConfig{Command: "first"}, Event: SessionStart, Scope: ScopeGlobal, Source: "/settings"}
	second := ResolvedHook{HookConfig: HookConfig{Command: "second"}, Event: SessionStart, Scope: ScopeGlobal, Source: "/settings"}
	selected := ResolveCompositionSelection(agentcomposition.Selection{Mode: agentcomposition.Select, Members: []agentcomposition.Module{{ID: "global:SessionStart:0"}}}, []ResolvedHook{first, second})
	d := agentcomposition.Builtins()[0]
	d.Hooks = selected
	got := CompositionHooks([]ResolvedHook{second, first}, d)
	if len(got) != 1 || got[0].Command != "first" {
		t.Fatalf("reordering changed the grant: %+v", got)
	}
	first.Command = "replacement"
	if got := CompositionHooks([]ResolvedHook{second, first}, d); len(got) != 0 {
		t.Fatalf("changed launch inherited an old grant: %+v", got)
	}
}
