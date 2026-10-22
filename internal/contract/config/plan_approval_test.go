package config

import (
	"slices"
	"strings"
	"testing"
)

func TestPlanApprovalIsAskUnlessExactlyAuto(t *testing.T) {
	for value, want := range map[string]bool{"": false, "ask": false, "auto": true, "Auto": true, " auto ": true, "yes": false, "autoo": false} {
		cfg := Default()
		cfg.Agent.PlanApproval = value
		if got := cfg.PlanApprovalAuto(); got != want {
			t.Errorf("plan_approval = %q -> auto %v, want %v", value, got, want)
		}
	}
	var none *Config
	if none.PlanApprovalAuto() {
		t.Error("a nil config releases the plan card")
	}
}

func TestUserFileChoosesPlanApproval(t *testing.T) {
	cfg, _ := loadScoped(t, "[agent]\nplan_approval = \"auto\"\n", "")
	if !cfg.PlanApprovalAuto() {
		t.Fatalf("a user file's auto was not honoured: %q", cfg.Agent.PlanApproval)
	}
}

func TestProjectCannotReleaseThePlanCard(t *testing.T) {
	cfg, _ := loadScoped(t, "", "[agent]\nplan_approval = \"auto\"\n")
	if cfg.PlanApprovalAuto() {
		t.Fatal("a cloned repo's reasonix.toml approved plans on the user's behalf")
	}
	if !slices.Contains(ignoredKeys(cfg), "agent.plan_approval") {
		t.Fatalf("ignored = %v, want agent.plan_approval reported", ignoredKeys(cfg))
	}
}

func TestProjectCannotTakeBackAnAutoPlanApproval(t *testing.T) {
	cfg, _ := loadScoped(t, "[agent]\nplan_approval = \"auto\"\n", "[agent]\nplan_approval = \"ask\"\n")
	if !cfg.PlanApprovalAuto() {
		t.Fatal("the project file overrode the user's own plan_approval")
	}
}

func TestAutoPlanApprovalSurvivesASave(t *testing.T) {
	cfg := Default()
	cfg.SetPlanApproval(true)
	if out := RenderTOMLForScope(cfg, RenderScopeUser); !strings.Contains(out, "plan_approval = \"auto\"") {
		t.Fatalf("rendered config lost the choice:\n%s", out)
	}
	if out := RenderTOMLForScope(Default(), RenderScopeUser); strings.Contains(out, "\nplan_approval = ") {
		t.Fatalf("the ask default was written as an active key:\n%s", out)
	}
	cfg.SetPlanApproval(false)
	if cfg.PlanApprovalAuto() || cfg.Agent.PlanApproval != "ask" {
		t.Fatalf("after SetPlanApproval(false): %q", cfg.Agent.PlanApproval)
	}
}
