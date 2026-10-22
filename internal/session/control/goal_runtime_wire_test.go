package control

import (
	"encoding/json"
	"testing"
)

func TestGoalRuntimeViewWireNamesWhatIsUsedAndNothingRetired(t *testing.T) {
	raw, err := json.Marshal(GoalRuntimeView{
		TurnsUsed: 3, TokensUsed: 900, RequestsUsed: 5, WorkDurationMs: 61000, NoProgressTurns: 1,
		LastReason: "tests fail", StopCause: "goal_stuck",
		TurnsLimit: 9, TokensLimit: 9, NoProgressLimit: 9, BudgetExtensions: 9,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"turnsUsed": 3.0, "tokensUsed": 900.0, "requestsUsed": 5.0, "workDurationMs": 61000.0, "noProgressTurns": 1.0,
		"lastReason": "tests fail", "stopCause": "goal_stuck",
	}
	if len(got) != len(want) {
		t.Fatalf("wire = %s, want exactly %v", raw, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
}
