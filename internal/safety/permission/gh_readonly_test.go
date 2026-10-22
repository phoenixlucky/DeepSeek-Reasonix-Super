package permission

import (
	"encoding/json"
	"testing"
)

func TestGhReadsAreReadOnlyAndGhWritesAreNot(t *testing.T) {
	for command, want := range map[string]bool{
		"gh pr checks 12":                       true,
		"gh api repos/o/r/actions/runs --jq .x": true,
		"gh pr view 12 --web":                   false,
		"gh api repos/o/r -X DELETE":            false,
		"gh pr merge 12":                        false,
		"gh run download 9":                     false,
		"gh pr list && gh pr create":            false,
		"gh pr list | head -3":                  true,
	} {
		args, _ := json.Marshal(map[string]any{"command": command})
		if got := BashCommandIsReadOnly(args); got != want {
			t.Errorf("BashCommandIsReadOnly(%q) = %v, want %v", command, got, want)
		}
	}
}
