package market

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestPluginSourcesMatchRegistryPublication(t *testing.T) {
	raw, err := os.ReadFile("testdata/plugin_sources.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases struct{ Accept, Reject []string }
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"plugin", "theme"} {
		for want, sources := range map[bool][]string{true: cases.Accept, false: cases.Reject} {
			for _, source := range sources {
				if got := sourceInstallable(kind, source) == nil; got != want {
					t.Errorf("sourceInstallable(%q, %q) accepted = %v, want %v", kind, source, got, want)
				}
				_, err := (Submission{Kind: kind, Name: "demo", Source: source}).Normalize()
				if got := err == nil; got != want {
					t.Errorf("publish(%q, %q) accepted = %v, want %v", kind, source, got, want)
				}
				if !want && (!errors.Is(err, ErrInvalidPluginSource) || !errors.Is(err, ErrUnpublishable) || !errors.Is(err, ErrBadSource)) {
					t.Errorf("publish(%q, %q) lost its refusal identity: %v", kind, source, err)
				}
			}
		}
	}
}
