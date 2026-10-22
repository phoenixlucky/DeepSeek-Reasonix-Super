package provider

import (
	"encoding/json"
	"testing"
)

func TestArgumentNormalizationIsLocalAndSurvivesProjection(t *testing.T) {
	const raw = `{"arguments":{"path":"a"}}`
	base := []Message{{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "1", Name: "read_file", Arguments: raw, ThoughtSignature: "signed"}}}}
	local := []Message{{Role: RoleAssistant, ToolCalls: []ToolCall{base[0].ToolCalls[0]}}}
	local[0].ToolCalls[0].ArgumentNormalization = &ArgumentNormalization{
		Reason: "redundant_arguments_envelope", Original: raw, Effective: `{"path":"a"}`,
	}
	before, _ := json.Marshal(ModelMessages(base))
	after, _ := json.Marshal(ModelMessages(local))
	if string(before) != string(after) {
		t.Fatalf("normalization changed provider bytes:\n%s\n%s", before, after)
	}
	projected := ProjectionMessages(local)
	if projected[0].ToolCalls[0].ArgumentNormalization == nil || local[0].ToolCalls[0].ArgumentNormalization == nil {
		t.Fatal("projection or model serialization erased durable metadata")
	}
	encoded, err := json.Marshal(local)
	if err != nil {
		t.Fatal(err)
	}
	var loaded []Message
	if err := json.Unmarshal(encoded, &loaded); err != nil {
		t.Fatal(err)
	}
	call := loaded[0].ToolCalls[0]
	if call.Arguments != raw || call.ExecutionArguments() != `{"path":"a"}` || call.ThoughtSignature != "signed" {
		t.Fatalf("round trip lost call identity: %+v", call)
	}
	if base[0].ToolCalls[0].ExecutionArguments() != raw {
		t.Fatal("old call without metadata changed")
	}
}
