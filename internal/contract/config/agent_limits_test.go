package config

import (
	"slices"
	"testing"
)

// Nothing bounds delegation by count until the user's own file says so; the one
// exception is the recursion depth, whose default is a named stop at 8 that only
// an explicit 0 removes.
func TestDelegationCountsAreUnlimitedByDefaultAndDepthHasAStop(t *testing.T) {
	d := Default().Agent
	if d.MaxSubagentConcurrency != 0 || d.MaxParallelWriters != 0 {
		t.Fatalf("defaults limit concurrency: %d sub-agents, %d writers", d.MaxSubagentConcurrency, d.MaxParallelWriters)
	}
	if d.MaxSubagentDepth != 8 {
		t.Fatalf("default depth = %d, want the stop at 8", d.MaxSubagentDepth)
	}
	cfg, _ := loadScoped(t, "", "")
	if cfg.Agent.MaxSubagentDepth != 8 {
		t.Fatalf("a config that says nothing has depth %d, want 8", cfg.Agent.MaxSubagentDepth)
	}
	cfg, _ = loadScoped(t, "[agent]\nmax_subagent_depth = 0\nmax_parallel_writers = 5\n", "")
	if cfg.Agent.MaxSubagentDepth != 0 || cfg.Agent.MaxParallelWriters != 5 {
		t.Fatalf("explicit user values were not kept: depth %d, writers %d", cfg.Agent.MaxSubagentDepth, cfg.Agent.MaxParallelWriters)
	}
}

func TestAProjectFileCannotMoveTheDelegationLimits(t *testing.T) {
	cfg, _ := loadScoped(t, "", "[agent]\nmax_subagent_concurrency = 1\nmax_parallel_writers = 1\nmax_subagent_depth = 0\n")
	if cfg.Agent.MaxSubagentConcurrency != 0 || cfg.Agent.MaxParallelWriters != 0 || cfg.Agent.MaxSubagentDepth != 8 {
		t.Fatalf("a cloned repo moved the limits: %+v", cfg.Agent)
	}
	for _, key := range []string{"agent.max_subagent_concurrency", "agent.max_parallel_writers", "agent.max_subagent_depth"} {
		if !slices.Contains(ignoredKeys(cfg), key) {
			t.Errorf("ignored = %v, want %s reported", ignoredKeys(cfg), key)
		}
	}
	cfg, _ = loadScoped(t, "[agent]\nmax_parallel_writers = 4\n", "[agent]\nmax_parallel_writers = 1\n")
	if cfg.Agent.MaxParallelWriters != 4 {
		t.Fatalf("the project file overrode the user's own limit: %d", cfg.Agent.MaxParallelWriters)
	}
}
