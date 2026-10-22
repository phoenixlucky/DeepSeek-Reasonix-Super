package config

import (
	"slices"
	"testing"
	"time"
)

func TestWriteLeaseMaxWaitIsOffUntilTheUserSetsIt(t *testing.T) {
	if got := Default().WriteLeaseMaxWait(); got != 0 {
		t.Fatalf("default = %v, want no limit: an automatic timeout is the user's choice", got)
	}
	cfg0, _ := loadScoped(t, "", "")
	if got := cfg0.WriteLeaseMaxWait(); got != 0 {
		t.Fatalf("absent key = %v, want no limit", got)
	}
	cfg, _ := loadScoped(t, "[agent]\nwrite_lease_max_wait_minutes = 0\n", "")
	if got := cfg.WriteLeaseMaxWait(); got != 0 {
		t.Fatalf("explicit 0 = %v, want no limit", got)
	}
	cfg, _ = loadScoped(t, "[agent]\nwrite_lease_max_wait_minutes = 15\n", "")
	if got := cfg.WriteLeaseMaxWait(); got != 15*time.Minute {
		t.Fatalf("15 = %v", got)
	}
	cfg, _ = loadScoped(t, "[agent]\nwrite_lease_max_wait_minutes = -3\n", "")
	if got := cfg.WriteLeaseMaxWait(); got != 0 {
		t.Fatalf("negative = %v, want no limit", got)
	}
}

func TestProjectCannotMoveTheWriteLeaseMaxWait(t *testing.T) {
	cfg, _ := loadScoped(t, "[agent]\nwrite_lease_max_wait_minutes = 120\n", "[agent]\nwrite_lease_max_wait_minutes = 1\n")
	if got := cfg.WriteLeaseMaxWait(); got != 120*time.Minute {
		t.Fatalf("a cloned repo's reasonix.toml moved the wait to %v", got)
	}
	if !slices.Contains(ignoredKeys(cfg), "agent.write_lease_max_wait_minutes") {
		t.Fatalf("ignored = %v, want the key reported", ignoredKeys(cfg))
	}
}
