package config

import (
	"slices"
	"testing"
)

func TestProjectCannotChangeGoalAutoRetry(t *testing.T) {
	cfg, _ := loadScoped(t, "[agent]\ngoal_auto_retry = 2\n", "[agent]\ngoal_auto_retry = 5\n")
	if cfg.GoalAutoRetryLimit() != 2 {
		t.Fatalf("the project file overrode the user's goal_auto_retry: %d", cfg.Agent.GoalAutoRetry)
	}
	if !slices.Contains(ignoredKeys(cfg), "agent.goal_auto_retry") {
		t.Fatalf("ignored = %v, want agent.goal_auto_retry reported", ignoredKeys(cfg))
	}
}

func TestProjectCannotTurnOnGoalAutoRetry(t *testing.T) {
	cfg, _ := loadScoped(t, "", "[agent]\ngoal_auto_retry = 3\n")
	if cfg.GoalAutoRetryLimit() != 0 {
		t.Fatal("a cloned repo's reasonix.toml switched the Goal retry on")
	}
}
