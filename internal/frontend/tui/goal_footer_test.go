package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/base/i18n"
)

func TestFooterShowsAGoalsStateAndUsage(t *testing.T) {
	m, _ := testModel(t)
	if got := strings.Join(footerPlainGroups(m), "  "); strings.Contains(got, i18n.M.ChatStatusGoalLabel) {
		t.Fatalf("a session with no goal has no goal group: %q", got)
	}
	if err := json.Unmarshal([]byte(`{"goal":"ship it","goalStatus":"running","goalRuntime":{"turnsUsed":4,"tokensUsed":128400,"requestsUsed":9,"workDurationMs":720000}}`), &m.status); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(footerPlainGroups(m), "  ")
	for _, want := range []string{i18n.M.ChatStatusGoalLabel, i18n.M.ChatStatusGoalRunning, "4", "128.4K", "12m"} {
		if !strings.Contains(got, want) {
			t.Fatalf("telemetry %q lacks %q", got, want)
		}
	}
}

func TestFooterNamesWhyAGoalStopped(t *testing.T) {
	m, _ := testModel(t)
	if err := json.Unmarshal([]byte(`{"goal":"ship it","goalStatus":"blocked","goalRuntime":{"turnsUsed":2,"stopCause":"goal_stuck"}}`), &m.status); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(footerPlainGroups(m), "  ")
	if !strings.Contains(got, i18n.M.ChatStatusGoalBlocked) || !strings.Contains(got, "goal_stuck") {
		t.Fatalf("a blocked goal must say so and why: %q", got)
	}
}
