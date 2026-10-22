package boot

import (
	"testing"

	"reasonix/internal/contract/event"
)

// What a frontend can say about a skill sub-agent's step, through the real
// assembly: the skill's name and what it was asked, the same as a task's row.
func TestEffectSkillSubagentDispatchCarriesItsTaskAsLabel(t *testing.T) {
	skills := map[string]string{"looker": "---\ndescription: looks\nrunAs: subagent\nread-only: true\n---\nLook.\n"}
	for _, tc := range []struct {
		call  scriptedCall
		label string
	}{
		{scriptedCall{"explore", `{"task":"Find where the loader is defined\nthen its callers"}`}, "Find where the loader is defined"},
		{scriptedCall{"run_skill", `{"name":"looker","arguments":"Locate the router\nand its tests"}`}, "Locate the router"},
		{scriptedCall{"read_only_skill", `{"name":"looker","arguments":"Read the config docs\nonly"}`}, "Read the config docs"},
	} {
		t.Run(tc.call.name, func(t *testing.T) {
			_, log := runScriptedSkillTurnWith(t, "skill-label-effect-"+tc.call.name, []scriptedCall{tc.call}, "go", skills, nil)
			log.mu.Lock()
			defer log.mu.Unlock()
			var got *event.Profile
			for _, e := range log.full {
				if e.Kind == event.ToolDispatch && e.Tool.Name == tc.call.name {
					got = e.Tool.Profile
				}
			}
			if got == nil {
				t.Fatalf("%s reached the sink with no profile", tc.call.name)
			}
			if got.Label != tc.label || got.Name == "" {
				t.Fatalf("profile = %+v, want a name and the label %q", got, tc.label)
			}
		})
	}
}
