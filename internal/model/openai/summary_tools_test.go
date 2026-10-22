package openai

import (
	"encoding/json"
	"testing"

	"reasonix/internal/contract/provider"
)

func TestSummaryKeepsSchemasButCannotChooseATool(t *testing.T) {
	c := &client{model: "m", extraBody: map[string]any{"tool_choice": "required"}}
	req := provider.Request{Tools: []provider.ToolSchema{{Name: "read_file"}}, Summary: true}
	raw, err := json.Marshal(c.buildRequest(req))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body["tool_choice"] != "none" || len(body["tools"].([]any)) != 1 {
		t.Fatalf("summary lost its schemas or allowed calls: %s", raw)
	}
	req.Summary = false
	raw, err = json.Marshal(c.buildRequest(req))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &body); err != nil || body["tool_choice"] != "required" {
		t.Fatalf("normal request no longer honors extra_body: %s, %v", raw, err)
	}
}
