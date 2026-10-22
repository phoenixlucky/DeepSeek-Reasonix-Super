package cli

import (
	"strings"
	"testing"

	"reasonix/internal/contract/agentpreset"
)

// The help and the refusal offer the settings agentpreset still has. A retired
// name keeps parsing for old scripts, but is no longer put forward as a choice:
// "light" runs Balanced, so offering it promises a policy nothing enforces.
func TestPresetHelpOffersOnlyTheLiveSettings(t *testing.T) {
	_, refusal := parseRuntimeProfile("fast")
	if refusal == nil {
		t.Fatal("an unknown setting was accepted")
	}
	for where, text := range map[string]string{
		"run --preset": newRunFlags().fs.Lookup("preset").Usage,
		"tui --preset": newTUIFlags().fs.Lookup("preset").Usage,
		"refusal":      refusal.Error(),
	} {
		for _, p := range agentpreset.All() {
			if !strings.Contains(text, string(p)) {
				t.Errorf("%s = %q, does not offer %q", where, text, p)
			}
		}
		if strings.Contains(text, "light") {
			t.Errorf("%s = %q, offers the retired light setting", where, text)
		}
	}
}
