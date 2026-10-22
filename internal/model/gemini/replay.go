package gemini

import (
	"encoding/json"
	"slices"
	"strings"

	"reasonix/internal/contract/provider"
)

// A turn's steps are stored as provider items and replayed verbatim while the
// message still matches them; a rewritten turn keeps its thoughts and rebuilds
// the rest.

type storedStep struct {
	raw  json.RawMessage
	typ  string
	id   string
	text string
}

func storedSteps(items []json.RawMessage) []storedStep {
	var out []storedStep
	for _, raw := range items {
		var probe struct {
			Type    string        `json:"type"`
			ID      string        `json:"id"`
			Content []textContent `json:"content"`
		}
		if json.Unmarshal(raw, &probe) != nil {
			continue
		}
		switch probe.Type {
		case "thought", "model_output", "function_call":
		default:
			continue
		}
		var text strings.Builder
		for _, c := range probe.Content {
			text.WriteString(c.Text)
		}
		out = append(out, storedStep{raw: raw, typ: probe.Type, id: probe.ID, text: text.String()})
	}
	return out
}

func assistantSteps(m provider.Message) []any {
	stored := storedSteps(m.ResponsesItems)
	if len(stored) > 0 && matches(stored, m) {
		out := make([]any, len(stored))
		for i, s := range stored {
			out[i] = s.raw
		}
		return out
	}
	var out []any
	for _, s := range stored {
		if s.typ == "thought" {
			out = append(out, s.raw)
		}
	}
	if m.Content != "" {
		out = append(out, contentStep{Type: "model_output", Content: []content{{Type: "text", Text: m.Content}}})
	}
	for _, tc := range m.ToolCalls {
		out = append(out, functionCallStep{Type: "function_call", ID: tc.ID, Name: tc.Name, Arguments: argumentsObject(tc.Arguments)})
	}
	return out
}

// matches says the stored steps still describe this message: the same calls in
// the same order, and the same answer text.
func matches(stored []storedStep, m provider.Message) bool {
	var ids []string
	var text strings.Builder
	for _, s := range stored {
		switch s.typ {
		case "function_call":
			ids = append(ids, s.id)
		case "model_output":
			text.WriteString(s.text)
		}
	}
	var want []string
	for _, tc := range m.ToolCalls {
		want = append(want, tc.ID)
	}
	return slices.Equal(ids, want) && text.String() == m.Content
}

// argumentsObject is the arguments a function_call step needs: a JSON object.
func argumentsObject(args string) json.RawMessage {
	args = strings.TrimSpace(args)
	var obj map[string]json.RawMessage
	if args == "" || json.Unmarshal([]byte(args), &obj) != nil || obj == nil {
		return json.RawMessage("{}")
	}
	return json.RawMessage(args)
}
