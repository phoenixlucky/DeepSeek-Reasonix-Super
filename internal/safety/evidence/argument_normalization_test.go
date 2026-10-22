package evidence

import (
	"testing"

	"reasonix/internal/contract/provider"
)

func TestPathsProvenUseNormalizedArguments(t *testing.T) {
	const raw = `{"arguments":{"path":"a.go"}}`
	msgs := []provider.Message{
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{
			ID: "r", Name: "read_file", Arguments: raw,
			ArgumentNormalization: &provider.ArgumentNormalization{
				Reason: provider.ArgumentEnvelopeUnwrapped, Original: raw, Effective: `{"path":"a.go"}`,
			},
		}}},
		{Role: provider.RoleTool, ToolCallID: "r", Content: "contents"},
	}
	facts := func(string) ToolFacts { return ToolFacts{ReadOnly: true} }
	if !PathsProvenInSession(msgs, []string{"a.go"}, false, facts) {
		t.Fatal("replay lost the normalized read path")
	}
	if PathsProvenInSession(msgs, []string{"b.go"}, false, facts) {
		t.Fatal("normalization invented evidence for an unread path")
	}
}
