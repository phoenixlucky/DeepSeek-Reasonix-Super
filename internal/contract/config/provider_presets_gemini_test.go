package config

import (
	"slices"
	"strings"
	"testing"
)

func geminiNativeEntry(t *testing.T, model string) *ProviderEntry {
	t.Helper()
	p, ok := CuratedProviderPreset("gemini-native")
	if !ok || len(p.Entries) != 1 {
		t.Fatalf("preset = %+v, %v", p, ok)
	}
	e := p.Entries[0]
	e.Model = model
	e.applyModelOverride()
	return &e
}

func TestTheNativeGeminiPresetSpeaksTheGeminiWireAndSaysItIsExperimental(t *testing.T) {
	p, _ := CuratedProviderPreset("gemini-native")
	e := p.Entries[0]
	if e.Kind != "gemini" || !SupportedProviderKind(e.Kind) || e.APIKeyEnv != "GEMINI_API_KEY" {
		t.Fatalf("entry = %+v", e)
	}
	if want := "experimental"; !containsFold(p.Label, want) || !containsFold(p.Description, want) {
		t.Fatalf("label %q / description %q must say the preset is experimental", p.Label, p.Description)
	}
	if other, _ := CuratedProviderPreset("gemini"); other.Entries[0].Kind != "openai" {
		t.Fatal("the OpenAI-compatible Gemini preset must stay as it was")
	}
}

func containsFold(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalFoldASCII(s[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}

func equalFoldASCII(a, b string) bool {
	for i := range a {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

// Each model below was read on its own page on 2026-10-17: 1,048,576 input tokens
// and Text, Image, Video, Audio and PDF input.
func TestEveryListedGeminiModelIsDeclaredToReadImagesAndVideo(t *testing.T) {
	for _, m := range geminiNativeModels {
		e := geminiNativeEntry(t, m)
		if !EffectiveVision(e) {
			t.Errorf("%s is not declared to read images", m)
		}
		if protocol, ok := EffectiveVideo(e); !ok || protocol != VideoProtocolGeminiFiles {
			t.Errorf("%s video = %q, %v", m, protocol, ok)
		}
	}
	for _, left := range []string{"gemini-3.1-flash-lite", "gemini-3-flash-preview", "gemini-3.7-flash"} {
		if slices.Contains(geminiNativeModels, left) {
			t.Errorf("%s is listed, but its page was not read", left)
		}
	}
}

// https://ai.google.dev/gemini-api/docs/thinking (read 2026-10-17).
func TestTheThinkingLevelsAreTheOnesEachModelDocuments(t *testing.T) {
	for model, want := range map[string][]string{
		"gemini-3.8-flash":       {"low", "medium", "high"},
		"gemini-4-argon":         {"low", "medium", "high"},
		"gemini-3.6-flash":       {"minimal", "low", "medium", "high"},
		"gemini-3.5-flash-lite":  {"minimal", "low", "medium", "high"},
		"gemini-3.1-pro-preview": {"low", "medium", "high"},
	} {
		got := RequestEffortLevels(geminiNativeEntry(t, model))
		if !slices.Equal(got, want) {
			t.Errorf("%s levels = %v, want %v", model, got, want)
		}
	}
}

func TestOnlyTheModelsWithAPublishedPriceCarryOne(t *testing.T) {
	e := geminiNativeEntry(t, "gemini-3.8-flash")
	for _, m := range []string{"gemini-3.8-flash", "gemini-3.6-flash", "gemini-3.5-flash-lite", "gemini-3.1-pro-preview"} {
		if e.Prices[m] == nil || e.Prices[m].Currency != "$" {
			t.Errorf("%s has no price: %+v", m, e.Prices[m])
		}
	}
	if e.Prices["gemini-4-argon"] != nil {
		t.Error("gemini-4-argon shows no price on Google's page, so none may be invented")
	}
}

func TestTheAnnotatedConfigSaysTheGeminiKindIsExperimental(t *testing.T) {
	var b strings.Builder
	renderProviderEntryAnnotated(&b, *geminiNativeEntry(t, "gemini-3.8-flash"))
	if !strings.Contains(b.String(), "experimental") {
		t.Fatalf("the config the user reads does not say so:\n%s", b.String())
	}
	b.Reset()
	renderProviderEntryAnnotated(&b, ProviderEntry{Name: "x", Kind: "openai", BaseURL: "https://x"})
	if strings.Contains(b.String(), "experimental") {
		t.Fatal("another kind was marked experimental")
	}
}
