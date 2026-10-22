package config

import (
	"slices"
	"strings"
	"testing"
)

func TestDefaultModeIsNormalUnlessExactlyPlan(t *testing.T) {
	for value, want := range map[string]bool{"": false, "normal": false, "plan": true, "Plan": true, " plan ": true, "goal": false, "planning": false} {
		cfg := Default()
		cfg.Agent.DefaultMode = value
		if got := cfg.DefaultPlanMode(); got != want {
			t.Errorf("default_mode = %q -> plan %v, want %v", value, got, want)
		}
	}
	var none *Config
	if none.DefaultPlanMode() {
		t.Error("a nil config starts conversations in plan mode")
	}
}

func TestSetDefaultModeAcceptsOnlyTheTwoModes(t *testing.T) {
	cfg := Default()
	if err := cfg.SetDefaultMode("plan"); err != nil || !cfg.DefaultPlanMode() {
		t.Fatalf("plan: err=%v plan=%v", err, cfg.DefaultPlanMode())
	}
	if err := cfg.SetDefaultMode("normal"); err != nil || cfg.DefaultPlanMode() {
		t.Fatalf("normal: err=%v plan=%v", err, cfg.DefaultPlanMode())
	}
	for _, bad := range []string{"goal", "auto", "Plan mode", ""} {
		if err := cfg.SetDefaultMode(bad); err == nil {
			t.Errorf("SetDefaultMode(%q) was accepted", bad)
		}
	}
}

func TestUserFileChoosesDefaultMode(t *testing.T) {
	cfg, _ := loadScoped(t, "[agent]\ndefault_mode = \"plan\"\n", "")
	if !cfg.DefaultPlanMode() {
		t.Fatalf("a user file's plan was not honoured: %q", cfg.Agent.DefaultMode)
	}
}

func TestProjectCannotChooseTheDefaultMode(t *testing.T) {
	cfg, _ := loadScoped(t, "", "[agent]\ndefault_mode = \"plan\"\n")
	if cfg.DefaultPlanMode() {
		t.Fatal("a cloned repo's reasonix.toml changed how the user's conversations start")
	}
	if !slices.Contains(ignoredKeys(cfg), "agent.default_mode") {
		t.Fatalf("ignored = %v, want agent.default_mode reported", ignoredKeys(cfg))
	}
}

func TestDefaultModeRendersOnlyWhenChosen(t *testing.T) {
	if out := RenderTOMLForScope(Default(), RenderScopeUser); strings.Contains(out, "\ndefault_mode = ") {
		t.Fatalf("the default rendered an active default_mode:\n%s", out)
	}
	cfg := Default()
	cfg.Agent.DefaultMode = "plan"
	if out := RenderTOMLForScope(cfg, RenderScopeUser); !strings.Contains(out, "default_mode = \"plan\"") {
		t.Fatalf("a chosen default_mode did not render:\n%s", out)
	}
}
