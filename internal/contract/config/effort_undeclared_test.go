package config

import (
	"strings"
	"testing"
)

func TestEffortUndeclaredSeparatesNothingDeclaredFromTurnedOff(t *testing.T) {
	entry := func(mutate func(*ProviderEntry)) *ProviderEntry {
		e := &ProviderEntry{Name: "relay", Kind: "responses", BaseURL: "https://relay.example/v1", Models: []string{"m"}, Model: "m"}
		mutate(e)
		return e
	}
	cases := []struct {
		name string
		e    *ProviderEntry
		want bool
	}{
		{"nothing declared", entry(func(*ProviderEntry) {}), true},
		{"levels declared", entry(func(e *ProviderEntry) { e.SupportedEfforts = []string{"low", "high", "max"} }), false},
		{"turned off by the protocol", entry(func(e *ProviderEntry) { e.ReasoningProtocol = ReasoningProtocolNone }), false},
		{"no entry", nil, false},
	}
	for _, tc := range cases {
		if got := EffortUndeclared(tc.e); got != tc.want {
			t.Errorf("%s: EffortUndeclared = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDeclaredLevelsOnAResponsesEntryMakeItConfigurable(t *testing.T) {
	e := &ProviderEntry{Name: "custom-opencode-ai", Kind: "responses", BaseURL: "https://opencode.ai/inference/go/v1",
		Models: []string{"deepseek-v4.1-flash"}, Model: "deepseek-v4.1-flash", SupportedEfforts: []string{"low", "high", "max"}}
	cap := EffortCapabilityForEntry(e)
	if !cap.Supported || len(cap.Levels) != 4 || cap.Levels[1] != "low" || cap.Levels[3] != "max" {
		t.Fatalf("capability = %+v, want auto plus the declared low, high, max", cap)
	}
}

func TestSettingALevelOnAnUndeclaredEntryNamesTheDeclaration(t *testing.T) {
	e := &ProviderEntry{Name: "relay", Kind: "responses", BaseURL: "https://relay.example/v1", Models: []string{"m"}, Model: "m"}
	if err := effortNotConfigurableError(e); err == nil || !contains(err.Error(), "supported_efforts") {
		t.Fatalf("error = %v, want it to name supported_efforts", err)
	}
	off := &ProviderEntry{Name: "relay", Kind: "responses", BaseURL: "https://relay.example/v1", Models: []string{"m"}, Model: "m", ReasoningProtocol: ReasoningProtocolNone}
	if err := effortNotConfigurableError(off); err == nil || contains(err.Error(), "supported_efforts") {
		t.Fatalf("error = %v, a protocol the user switched off is not an undeclared entry", err)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
