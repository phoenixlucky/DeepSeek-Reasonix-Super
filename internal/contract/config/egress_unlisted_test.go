package config

import (
	"slices"
	"strings"
	"testing"
)

func TestEgressUnlistedOnlyAllowMeansAllow(t *testing.T) {
	for value, want := range map[string]bool{"": false, "ask": false, "allow": true, " ALLOW ": true, "yes": false, "true": false} {
		cfg := &Config{}
		cfg.Sandbox.EgressUnlisted = value
		if got := cfg.EgressUnlistedAllowed(); got != want {
			t.Errorf("egress_unlisted = %q -> %v, want %v", value, got, want)
		}
	}
	var none *Config
	if none.EgressUnlistedAllowed() {
		t.Error("a nil config skips the question")
	}
}

func TestUserFileChoosesEgressUnlisted(t *testing.T) {
	cfg, _ := loadScoped(t, "[sandbox]\negress_unlisted = \"allow\"\n", "")
	if !cfg.EgressUnlistedAllowed() {
		t.Fatalf("a user file's allow was not honoured: %q", cfg.Sandbox.EgressUnlisted)
	}
}

func TestProjectCannotSwitchOffItsOwnEgressQuestion(t *testing.T) {
	cfg, _ := loadScoped(t, "", "[sandbox]\negress_unlisted = \"allow\"\n")
	if cfg.EgressUnlistedAllowed() {
		t.Fatal("a cloned repo's reasonix.toml stopped the egress question on the user's behalf")
	}
	if !slices.Contains(ignoredKeys(cfg), "sandbox.egress_unlisted") {
		t.Fatalf("ignored = %v, want sandbox.egress_unlisted reported", ignoredKeys(cfg))
	}
}

func TestRenderCarriesEgressUnlisted(t *testing.T) {
	cfg := &Config{}
	cfg.Sandbox.EgressUnlisted = "allow"
	var b strings.Builder
	renderDomainLists(&b, cfg.Sandbox)
	if got := b.String(); got == "" || got[:len("egress_unlisted")] != "egress_unlisted" {
		t.Fatalf("render = %q", got)
	}
}
